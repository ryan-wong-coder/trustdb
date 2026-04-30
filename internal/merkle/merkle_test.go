package merkle

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ryan-wong-coder/trustdb/internal/model"
)

func TestBuildProofAndVerify(t *testing.T) {
	t.Parallel()

	records := []model.ServerRecord{
		record("a"),
		record("b"),
		record("c"),
		record("d"),
		record("e"),
	}
	tree, err := Build(records)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	root := tree.Root()
	if len(root) == 0 {
		t.Fatal("Build() returned empty root")
	}

	for i := range records {
		i := i
		t.Run(records[i].RecordID, func(t *testing.T) {
			t.Parallel()
			leaf, err := tree.LeafHash(i)
			if err != nil {
				t.Fatalf("LeafHash() error = %v", err)
			}
			proof, err := tree.Proof(i)
			if err != nil {
				t.Fatalf("Proof() error = %v", err)
			}
			if !Verify(leaf, uint64(i), uint64(len(records)), proof, root) {
				t.Fatalf("Verify() = false for leaf %d", i)
			}
		})
	}
}

func TestBuildProofAndVerifyManySizes(t *testing.T) {
	t.Parallel()

	for _, size := range []int{1, 2, 3, 4, 5, 8, 16, 31, 32, 33, 1024} {
		size := size
		t.Run(fmt.Sprintf("n=%d", size), func(t *testing.T) {
			t.Parallel()

			records := make([]model.ServerRecord, size)
			for i := range records {
				records[i] = record(fmt.Sprintf("rec-%04d", i))
			}
			tree, err := Build(records)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			root := tree.Root()
			if len(root) == 0 {
				t.Fatal("Build() returned empty root")
			}
			proofs := tree.Proofs()
			if len(proofs) != size {
				t.Fatalf("Proofs() len = %d, want %d", len(proofs), size)
			}
			for i := range records {
				leaf, err := tree.LeafHash(i)
				if err != nil {
					t.Fatalf("LeafHash(%d) error = %v", i, err)
				}
				if !Verify(leaf, uint64(i), uint64(len(records)), proofs[i], root) {
					t.Fatalf("Verify() = false for leaf %d of %d", i, size)
				}
			}
		})
	}
}

func TestVerifyRejectsWrongRoot(t *testing.T) {
	t.Parallel()

	records := []model.ServerRecord{record("a"), record("b"), record("c")}
	tree, err := Build(records)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	leaf, err := tree.LeafHash(1)
	if err != nil {
		t.Fatalf("LeafHash() error = %v", err)
	}
	proof, err := tree.Proof(1)
	if err != nil {
		t.Fatalf("Proof() error = %v", err)
	}
	badRoot := bytes.Repeat([]byte{9}, 32)
	if Verify(leaf, 1, uint64(len(records)), proof, badRoot) {
		t.Fatal("Verify() = true, want false for bad root")
	}
}

func TestBuildRejectsEmptyRecords(t *testing.T) {
	t.Parallel()

	if _, err := Build(nil); err == nil {
		t.Fatal("Build(nil) error = nil, want error")
	}
}

func record(id string) model.ServerRecord {
	return model.ServerRecord{
		SchemaVersion:       model.SchemaServerRecord,
		RecordID:            id,
		TenantID:            "tenant",
		ClientID:            "client",
		KeyID:               "key",
		ClaimHash:           bytes.Repeat([]byte(id), 32)[:32],
		ClientSignatureHash: bytes.Repeat([]byte{1}, 32),
		ReceivedAtUnixN:     100,
		Validation: model.Validation{
			PolicyVersion:       model.DefaultValidationPolicy,
			HashAlgAllowed:      true,
			SignatureAlgAllowed: true,
			KeyStatus:           "valid",
		},
	}
}
