package main

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowtrust/trustdb/internal/cborx"
	"github.com/wowtrust/trustdb/internal/cryptosuite"
	"github.com/wowtrust/trustdb/internal/model"
)

func TestReadGlobalProofFileExplainsAnchorResultMixup(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sth-1.tdanchor-result")
	writeCBORForTest(t, path, model.STHAnchorResult{
		SchemaVersion: model.SchemaSTHAnchorResult,
		TreeSize:      1,
		SinkName:      "ots",
		AnchorID:      "ots-test",
		RootHash:      []byte{1, 2, 3},
		STH: model.SignedTreeHead{
			SchemaVersion: model.SchemaSignedTreeHead,
			TreeSize:      1,
			RootHash:      []byte{1, 2, 3},
		},
	})

	var proof model.GlobalLogProof
	err := readGlobalProofFile(path, &proof)
	if err == nil {
		t.Fatal("readGlobalProofFile() error = nil, want type hint")
	}
	msg := err.Error()
	if !strings.Contains(msg, "STHAnchorResult") || !strings.Contains(msg, ".tdanchor-result") || !strings.Contains(msg, ".tdgproof") {
		t.Fatalf("error message = %q, want actionable file type hint", msg)
	}
}

func TestDesktopAnchorPluginArgs(t *testing.T) {
	t.Parallel()

	got := desktopAnchorPluginArgs("--network\r\n consortium-a \n\n--strict")
	want := []string{"--network", "consortium-a", "--strict"}
	if len(got) != len(want) {
		t.Fatalf("desktopAnchorPluginArgs() = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("desktopAnchorPluginArgs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadGlobalProofFileExplainsLegacyBatchAnchorMixup(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "batch-old.tdanchor-result")
	writeCBORForTest(t, path, map[string]any{
		"anchor_id":  "ots-old",
		"batch_root": []byte{1, 2, 3},
		"proof":      []byte(`{"schema_version":"trustdb.anchor-ots-proof.v1"}`),
	})

	var proof model.GlobalLogProof
	err := readGlobalProofFile(path, &proof)
	if err == nil {
		t.Fatal("readGlobalProofFile() error = nil, want legacy hint")
	}
	msg := err.Error()
	if !strings.Contains(msg, "legacy batch anchor") || !strings.Contains(msg, "GlobalLogProof") {
		t.Fatalf("error message = %q, want legacy batch-anchor hint", msg)
	}
}

func TestReadSingleProofFileRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sample.sproof")
	writeCBORForTest(t, path, model.SingleProof{
		SchemaVersion:   model.SchemaSingleProof,
		FormatVersion:   2,
		CryptoSuite:     cryptosuite.INTLV1,
		RecordID:        "rec-1",
		ProofLevel:      "L3",
		NodeID:          "node-1",
		LogID:           "log-1",
		ExportedAtUnixN: 1,
		ProofBundle: model.ProofBundle{
			SchemaVersion: model.SchemaProofBundle,
			CryptoSuite:   cryptosuite.INTLV1,
			RecordID:      "rec-1",
			NodeID:        "node-1",
			LogID:         "log-1",
		},
	})

	var proof model.SingleProof
	if err := readSingleProofFile(path, &proof); err != nil {
		t.Fatalf("readSingleProofFile() error = %v", err)
	}
	if proof.SchemaVersion != model.SchemaSingleProof || proof.RecordID != "rec-1" || proof.ProofBundle.RecordID != "rec-1" {
		t.Fatalf("decoded single proof = %+v, want bundled artefacts", proof)
	}
}

func TestReadProofBundleFileExplainsSingleProofMixup(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sample.sproof")
	writeCBORForTest(t, path, model.SingleProof{
		SchemaVersion: model.SchemaSingleProof,
		FormatVersion: 2,
		CryptoSuite:   cryptosuite.INTLV1,
		RecordID:      "rec-1",
		ProofLevel:    "L3",
		NodeID:        "node-1",
		LogID:         "log-1",
		ProofBundle: model.ProofBundle{
			SchemaVersion: model.SchemaProofBundle,
			CryptoSuite:   cryptosuite.INTLV1,
			RecordID:      "rec-1",
			NodeID:        "node-1",
			LogID:         "log-1",
		},
	})

	var bundle model.ProofBundle
	err := readProofBundleFile(path, &bundle)
	if err == nil {
		t.Fatal("readProofBundleFile() error = nil, want single-proof hint")
	}
	msg := err.Error()
	if !strings.Contains(msg, ".sproof") || !strings.Contains(msg, "main .sproof input") {
		t.Fatalf("error message = %q, want single-proof hint", msg)
	}
}

func TestReadProofBundleFileRejectsOversizedInput(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "oversized.tdproof")
	if err := os.WriteFile(path, make([]byte, cborx.DefaultMaxBytes+1), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var bundle model.ProofBundle
	err := readProofBundleFile(path, &bundle)
	if err == nil || !strings.Contains(err.Error(), "payload too large") {
		t.Fatalf("readProofBundleFile() error = %v, want payload too large", err)
	}
}

func TestDesktopTrustedKeysPreserveRotatedSignatureKeyIDs(t *testing.T) {
	t.Parallel()

	clientPublic, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	serverPublic, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	bundle := model.ProofBundle{
		SignedClaim: model.SignedClaim{Signature: model.Signature{KeyID: "client-key"}},
		AcceptedReceipt: model.AcceptedReceipt{
			ServerSig: model.Signature{KeyID: "accepted-key"},
		},
		CommittedReceipt: model.CommittedReceipt{
			ServerSig: model.Signature{KeyID: "committed-key"},
		},
	}
	global := &model.GlobalLogProof{
		STH: model.SignedTreeHead{Signature: model.Signature{KeyID: "sth-key"}},
	}
	keys, err := desktopTrustedKeys(bundle, clientPublic, serverPublic, global)
	if err != nil {
		t.Fatal(err)
	}
	for name, descriptor := range map[string]struct {
		got  string
		want string
	}{
		"client":    {got: keys.ClientPublicKey.KeyID, want: "client-key"},
		"accepted":  {got: keys.AcceptedReceiptPublicKey.KeyID, want: "accepted-key"},
		"committed": {got: keys.CommittedReceiptPublicKey.KeyID, want: "committed-key"},
		"STH":       {got: keys.SignedTreeHeadPublicKey.KeyID, want: "sth-key"},
	} {
		if descriptor.got != descriptor.want {
			t.Fatalf("%s key ID = %q, want %q", name, descriptor.got, descriptor.want)
		}
	}
	if keys.ClientPublicKey.CryptoSuite != cryptosuite.INTLV1 ||
		keys.SignedTreeHeadPublicKey.CryptoSuite != cryptosuite.INTLV1 {
		t.Fatalf("desktop trust suites = client:%s STH:%s", keys.ClientPublicKey.CryptoSuite, keys.SignedTreeHeadPublicKey.CryptoSuite)
	}
}

func writeCBORForTest(t *testing.T, path string, v any) {
	t.Helper()
	data, err := cborx.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
