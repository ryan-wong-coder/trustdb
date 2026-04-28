package verify

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"io"

	"github.com/ryan-wong-coder/trustdb/internal/anchor"
	"github.com/ryan-wong-coder/trustdb/internal/claim"
	"github.com/ryan-wong-coder/trustdb/internal/globallog"
	"github.com/ryan-wong-coder/trustdb/internal/merkle"
	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/prooflevel"
	"github.com/ryan-wong-coder/trustdb/internal/receipt"
	"github.com/ryan-wong-coder/trustdb/internal/trustcrypto"
)

type TrustedKeys struct {
	ClientPublicKey ed25519.PublicKey
	ServerPublicKey ed25519.PublicKey
}

// Result is the outcome of ProofBundle. ProofLevel follows the centralized
// prooflevel ladder so CLI, desktop and future SDK code do not drift.
type Result struct {
	Valid      bool   `json:"valid"`
	RecordID   string `json:"record_id"`
	ProofLevel string `json:"proof_level"`
	// AnchorSink is populated only when an anchor was verified; it
	// identifies the sink ("file", "noop", ...) that produced the
	// AnchorResult so downstream tooling knows how to interpret
	// the Proof bytes.
	AnchorSink string `json:"anchor_sink,omitempty"`
	AnchorID   string `json:"anchor_id,omitempty"`
}

// Option tunes ProofBundle without growing its positional signature
// every time a new trust layer is added. Every option is optional;
// the zero-option call verifies up to L3.
type Option func(*options)

type options struct {
	global *model.GlobalLogProof
	anchor *model.STHAnchorResult
}

func WithGlobalProof(p model.GlobalLogProof) Option {
	return func(o *options) { o.global = &p }
}

// WithAnchor asks ProofBundle to additionally verify an L5 external STH
// anchor. L5 requires a matching WithGlobalProof because batch roots are no
// longer directly anchored.
func WithAnchor(a model.STHAnchorResult) Option {
	return func(o *options) { o.anchor = &a }
}

func ProofBundle(raw io.Reader, bundle model.ProofBundle, keys TrustedKeys, opts ...Option) (Result, error) {
	var o options
	for _, apply := range opts {
		apply(&o)
	}
	if bundle.SchemaVersion != model.SchemaProofBundle {
		return Result{}, fmt.Errorf("verify: unexpected proof bundle schema: %s", bundle.SchemaVersion)
	}
	sum, n, err := trustcrypto.HashReader(bundle.SignedClaim.Claim.Content.HashAlg, raw)
	if err != nil {
		return Result{}, err
	}
	if n != bundle.SignedClaim.Claim.Content.ContentLength {
		return Result{}, fmt.Errorf("verify: content length mismatch: got %d want %d", n, bundle.SignedClaim.Claim.Content.ContentLength)
	}
	if !bytes.Equal(sum, bundle.SignedClaim.Claim.Content.ContentHash) {
		return Result{}, fmt.Errorf("verify: content hash mismatch")
	}
	verified, err := claim.Verify(bundle.SignedClaim, keys.ClientPublicKey)
	if err != nil {
		return Result{}, err
	}
	if verified.RecordID != bundle.RecordID || verified.RecordID != bundle.ServerRecord.RecordID {
		return Result{}, fmt.Errorf("verify: record id mismatch")
	}
	if err := receipt.VerifyAccepted(bundle.AcceptedReceipt, keys.ServerPublicKey); err != nil {
		return Result{}, err
	}
	if err := receipt.VerifyCommitted(bundle.CommittedReceipt, keys.ServerPublicKey); err != nil {
		return Result{}, err
	}
	leaf, err := merkle.HashLeaf(bundle.ServerRecord)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(leaf, bundle.CommittedReceipt.LeafHash) {
		return Result{}, fmt.Errorf("verify: leaf hash mismatch")
	}
	if !merkle.Verify(
		leaf,
		bundle.BatchProof.LeafIndex,
		bundle.BatchProof.TreeSize,
		bundle.BatchProof.AuditPath,
		bundle.CommittedReceipt.BatchRoot,
	) {
		return Result{}, fmt.Errorf("verify: merkle proof failed")
	}
	evidence := prooflevel.EvidenceFor(prooflevel.L3)
	result := Result{
		Valid:      true,
		RecordID:   verified.RecordID,
		ProofLevel: prooflevel.Evaluate(evidence).String(),
	}
	if o.global != nil {
		if err := GlobalLogConsistency(bundle, *o.global); err != nil {
			return Result{}, err
		}
		evidence.GlobalLogProof = true
		result.ProofLevel = prooflevel.Evaluate(evidence).String()
	}
	if o.anchor != nil {
		if o.global == nil {
			return Result{}, fmt.Errorf("verify: L5 anchor requires a global log proof")
		}
		if err := AnchorConsistency(*o.global, *o.anchor); err != nil {
			return Result{}, err
		}
		evidence.STHAnchorResult = true
		result.ProofLevel = prooflevel.Evaluate(evidence).String()
		result.AnchorSink = o.anchor.SinkName
		result.AnchorID = o.anchor.AnchorID
	}
	return result, nil
}

func GlobalLogConsistency(bundle model.ProofBundle, proof model.GlobalLogProof) error {
	if proof.SchemaVersion != model.SchemaGlobalLogProof {
		return fmt.Errorf("verify: unexpected global log proof schema: %s", proof.SchemaVersion)
	}
	if proof.BatchID != bundle.CommittedReceipt.BatchID {
		return fmt.Errorf("verify: global proof batch_id mismatch: proof=%s bundle=%s", proof.BatchID, bundle.CommittedReceipt.BatchID)
	}
	if !globallog.VerifyInclusion(proof) {
		return fmt.Errorf("verify: global log inclusion proof failed")
	}
	leaf := model.GlobalLogLeaf{
		SchemaVersion:      model.SchemaGlobalLogLeaf,
		BatchID:            bundle.CommittedReceipt.BatchID,
		BatchRoot:          bundle.CommittedReceipt.BatchRoot,
		BatchTreeSize:      bundle.BatchProof.TreeSize,
		BatchClosedAtUnixN: bundle.CommittedReceipt.ClosedAtUnixN,
		LeafIndex:          proof.LeafIndex,
	}
	hash, err := globallog.HashLeaf(leaf)
	if err != nil {
		return err
	}
	if !bytes.Equal(hash, proof.LeafHash) {
		return fmt.Errorf("verify: global log leaf hash mismatch")
	}
	return nil
}

// AnchorConsistency checks that an STHAnchorResult is bound to the same STH
// proven by the supplied global log proof. It does not talk to the external
// sink; sink-specific Proof bytes are verified by sink-aware tooling.
func AnchorConsistency(proof model.GlobalLogProof, ar model.STHAnchorResult) error {
	if ar.SchemaVersion != model.SchemaSTHAnchorResult {
		return fmt.Errorf("verify: unexpected anchor result schema: %s", ar.SchemaVersion)
	}
	if ar.TreeSize != proof.STH.TreeSize {
		return fmt.Errorf("verify: anchor tree_size mismatch: anchor=%d sth=%d", ar.TreeSize, proof.STH.TreeSize)
	}
	if !bytes.Equal(ar.RootHash, proof.STH.RootHash) {
		return fmt.Errorf("verify: anchor root_hash does not match STH root")
	}
	if ar.AnchorID == "" {
		return fmt.Errorf("verify: anchor result is missing anchor_id")
	}
	switch ar.SinkName {
	case anchor.FileSinkName:
		if got, want := ar.AnchorID, anchor.DeterministicFileAnchorID(proof.STH); got != want {
			return fmt.Errorf("verify: file sink anchor_id mismatch: got %s want %s", got, want)
		}
	case anchor.NoopSinkName:
		if got, want := ar.AnchorID, anchor.DeterministicNoopAnchorID(proof.STH); got != want {
			return fmt.Errorf("verify: noop sink anchor_id mismatch: got %s want %s", got, want)
		}
	default:
		// Unknown sink: trust the sink's own verifier to handle it.
	}
	return nil
}
