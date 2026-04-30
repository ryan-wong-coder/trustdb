package app

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/ryan-wong-coder/trustdb/internal/cborx"
	"github.com/ryan-wong-coder/trustdb/internal/claim"
	"github.com/ryan-wong-coder/trustdb/internal/merkle"
	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/receipt"
	"github.com/ryan-wong-coder/trustdb/internal/trustcrypto"
	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
	"github.com/ryan-wong-coder/trustdb/internal/wal"
)

type LocalEngine struct {
	ServerID string
	// LogID scopes batch and transparency-log identifiers for this compute node (shared proofstore).
	LogID            string
	ServerKeyID      string
	ClientPublicKey  ed25519.PublicKey
	ClientKeys       ClientKeyResolver
	ServerPrivateKey ed25519.PrivateKey
	WAL              *wal.Writer
	Idempotency      *IdempotencyIndex
	Now              func() time.Time
}

type ClientKeyResolver interface {
	LookupClientKeyAt(tenantID, clientID, keyID string, at time.Time) (model.ClientKey, error)
}

type ReplayedAccepted struct {
	Signed   model.SignedClaim
	Record   model.ServerRecord
	Accepted model.AcceptedReceipt
}

// Submit validates a signed claim, appends it to the WAL on first submission,
// and returns the resulting server record and accepted receipt. The
// idempotent return value is true when the claim matches an existing
// idempotency_key entry and the returned pair was generated on a previous
// submission; callers forwarding to downstream pipelines (e.g. batch
// enqueue) should skip idempotent replays to avoid duplicate work. Conflicts
// on (tenant_id, client_id, idempotency_key) with a different claim body are
// surfaced as CodeAlreadyExists.
func (e LocalEngine) Submit(ctx context.Context, signed model.SignedClaim) (model.ServerRecord, model.AcceptedReceipt, bool, error) {
	if e.WAL == nil {
		return model.ServerRecord{}, model.AcceptedReceipt{}, false, fmt.Errorf("app: WAL writer is nil")
	}
	now := e.now()
	verified, keyStatus, claimHash, sigHash, err := e.validateSigned(signed, now)
	if err != nil {
		return model.ServerRecord{}, model.AcceptedReceipt{}, false, err
	}
	idemKey := IdempotencyKey(signed.Claim.TenantID, signed.Claim.ClientID, signed.Claim.IdempotencyKey)
	build := func() (model.ServerRecord, model.AcceptedReceipt, error) {
		payload, err := cborx.Marshal(signed)
		if err != nil {
			return model.ServerRecord{}, model.AcceptedReceipt{}, err
		}
		pos, _, err := e.WAL.AppendAt(ctx, payload, now)
		if err != nil {
			return model.ServerRecord{}, model.AcceptedReceipt{}, err
		}
		return e.buildAccepted(signed, verified, keyStatus, claimHash, sigHash, pos, now)
	}
	if e.Idempotency == nil || idemKey == "" {
		record, accepted, err := build()
		return record, accepted, false, err
	}
	record, accepted, loaded, conflict, err := e.Idempotency.Remember(idemKey, claimHash, build)
	if err != nil {
		return model.ServerRecord{}, model.AcceptedReceipt{}, false, err
	}
	if conflict {
		return model.ServerRecord{}, model.AcceptedReceipt{}, false, trusterr.New(
			trusterr.CodeAlreadyExists,
			fmt.Sprintf("idempotency_key %q already associated with a different claim", signed.Claim.IdempotencyKey),
		)
	}
	return record, accepted, loaded, nil
}

func (e LocalEngine) ReplayAccepted(record wal.Record) (ReplayedAccepted, error) {
	var signed model.SignedClaim
	if err := cborx.UnmarshalLimit(record.Payload, &signed, len(record.Payload)); err != nil {
		return ReplayedAccepted{}, err
	}
	receivedAt := time.Unix(0, record.UnixNano).UTC()
	verified, keyStatus, claimHash, sigHash, err := e.validateSigned(signed, receivedAt)
	if err != nil {
		return ReplayedAccepted{}, err
	}
	serverRecord, accepted, err := e.buildAccepted(
		signed,
		verified,
		keyStatus,
		claimHash,
		sigHash,
		record.Position,
		receivedAt,
	)
	if err != nil {
		return ReplayedAccepted{}, err
	}
	return ReplayedAccepted{
		Signed:   signed,
		Record:   serverRecord,
		Accepted: accepted,
	}, nil
}

func (e LocalEngine) validateSigned(signed model.SignedClaim, receivedAt time.Time) (claim.Verified, string, []byte, []byte, error) {
	clientPub, keyStatus, err := e.resolveClientKey(signed, receivedAt)
	if err != nil {
		return claim.Verified{}, "", nil, nil, err
	}
	verified, err := claim.Verify(signed, clientPub)
	if err != nil {
		return claim.Verified{}, "", nil, nil, err
	}
	claimHash, err := trustcrypto.HashBytes(model.DefaultHashAlg, verified.ClaimCBOR)
	if err != nil {
		return claim.Verified{}, "", nil, nil, err
	}
	sigHash, err := trustcrypto.HashBytes(model.DefaultHashAlg, signed.Signature.Signature)
	if err != nil {
		return claim.Verified{}, "", nil, nil, err
	}
	return verified, keyStatus, claimHash, sigHash, nil
}

func (e LocalEngine) buildAccepted(
	signed model.SignedClaim,
	verified claim.Verified,
	keyStatus string,
	claimHash []byte,
	sigHash []byte,
	pos model.WALPosition,
	receivedAt time.Time,
) (model.ServerRecord, model.AcceptedReceipt, error) {
	record := model.ServerRecord{
		SchemaVersion:       model.SchemaServerRecord,
		RecordID:            verified.RecordID,
		TenantID:            signed.Claim.TenantID,
		ClientID:            signed.Claim.ClientID,
		KeyID:               signed.Claim.KeyID,
		ClaimHash:           claimHash,
		ClientSignatureHash: sigHash,
		ReceivedAtUnixN:     receivedAt.UnixNano(),
		WAL:                 pos,
		Validation: model.Validation{
			PolicyVersion:       model.DefaultValidationPolicy,
			HashAlgAllowed:      true,
			SignatureAlgAllowed: true,
			KeyStatus:           keyStatus,
		},
	}
	accepted := model.AcceptedReceipt{
		SchemaVersion:   model.SchemaAcceptedReceipt,
		RecordID:        record.RecordID,
		Status:          "accepted",
		ServerID:        e.ServerID,
		ReceivedAtUnixN: receivedAt.UnixNano(),
		WAL:             pos,
	}
	accepted, err := receipt.SignAccepted(accepted, e.ServerKeyID, e.ServerPrivateKey)
	if err != nil {
		return model.ServerRecord{}, model.AcceptedReceipt{}, err
	}
	return record, accepted, nil
}

func (e LocalEngine) resolveClientKey(signed model.SignedClaim, receivedAt time.Time) (ed25519.PublicKey, string, error) {
	if e.ClientKeys != nil {
		key, err := e.ClientKeys.LookupClientKeyAt(
			signed.Claim.TenantID,
			signed.Claim.ClientID,
			signed.Claim.KeyID,
			receivedAt,
		)
		if err != nil {
			return nil, "", err
		}
		if key.Alg != model.DefaultSignatureAlg {
			return nil, "", fmt.Errorf("app: unsupported client key alg: %s", key.Alg)
		}
		if len(key.PublicKey) != ed25519.PublicKeySize {
			return nil, "", fmt.Errorf("app: invalid resolved client public key size: %d", len(key.PublicKey))
		}
		return ed25519.PublicKey(key.PublicKey), key.Status, nil
	}
	if len(e.ClientPublicKey) != ed25519.PublicKeySize {
		return nil, "", fmt.Errorf("app: client public key or key resolver required")
	}
	return e.ClientPublicKey, model.KeyStatusValid, nil
}

func (e LocalEngine) CommitBatch(batchID string, closedAt time.Time, signed []model.SignedClaim, records []model.ServerRecord, accepted []model.AcceptedReceipt) ([]model.ProofBundle, error) {
	return e.commitBatch(batchID, closedAt, signed, records, accepted, true)
}

func (e LocalEngine) CommitBatchIndexes(batchID string, closedAt time.Time, signed []model.SignedClaim, records []model.ServerRecord, accepted []model.AcceptedReceipt) (model.BatchRoot, []model.RecordIndex, error) {
	bundles, err := e.commitBatch(batchID, closedAt, signed, records, accepted, false)
	if err != nil {
		return model.BatchRoot{}, nil, err
	}
	root := model.BatchRoot{
		SchemaVersion: model.SchemaBatchRoot,
		BatchID:       batchID,
		NodeID:        e.ServerID,
		LogID:         e.LogID,
		BatchRoot:     append([]byte(nil), bundles[0].CommittedReceipt.BatchRoot...),
		TreeSize:      uint64(len(bundles)),
		ClosedAtUnixN: bundles[0].CommittedReceipt.ClosedAtUnixN,
	}
	indexes := make([]model.RecordIndex, len(bundles))
	for i := range bundles {
		indexes[i] = model.RecordIndexFromBundle(bundles[i])
	}
	return root, indexes, nil
}

func (e LocalEngine) commitBatch(batchID string, closedAt time.Time, signed []model.SignedClaim, records []model.ServerRecord, accepted []model.AcceptedReceipt, includeProofs bool) ([]model.ProofBundle, error) {
	if len(records) == 0 || len(records) != len(signed) || len(records) != len(accepted) {
		return nil, fmt.Errorf("app: inconsistent batch input sizes")
	}
	tree, err := merkle.Build(records)
	if err != nil {
		return nil, err
	}
	if closedAt.IsZero() {
		closedAt = e.now()
	}
	closedAt = closedAt.UTC()
	root := tree.Root()
	var proofs [][][]byte
	if includeProofs {
		proofs = tree.Proofs()
	}
	bundles := make([]model.ProofBundle, len(records))
	for i := range records {
		leaf, err := tree.LeafHash(i)
		if err != nil {
			return nil, err
		}
		committed := model.CommittedReceipt{
			SchemaVersion: model.SchemaCommittedReceipt,
			RecordID:      records[i].RecordID,
			Status:        "committed",
			BatchID:       batchID,
			LeafIndex:     uint64(i),
			LeafHash:      leaf,
			BatchRoot:     append([]byte(nil), root...),
			ClosedAtUnixN: closedAt.UnixNano(),
			NodeID:        e.ServerID,
			LogID:         e.LogID,
		}
		committed, err = receipt.SignCommitted(committed, e.ServerKeyID, e.ServerPrivateKey)
		if err != nil {
			return nil, err
		}
		bundles[i] = model.ProofBundle{
			SchemaVersion:    model.SchemaProofBundle,
			RecordID:         records[i].RecordID,
			NodeID:           e.ServerID,
			LogID:            e.LogID,
			SignedClaim:      signed[i],
			ServerRecord:     records[i],
			AcceptedReceipt:  accepted[i],
			CommittedReceipt: committed,
			BatchProof: model.BatchProof{
				TreeAlg:   model.DefaultMerkleTreeAlg,
				LeafIndex: uint64(i),
				TreeSize:  uint64(len(records)),
			},
		}
		if includeProofs {
			bundles[i].BatchProof.AuditPath = proofs[i]
		}
	}
	return bundles, nil
}

func (e LocalEngine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}
