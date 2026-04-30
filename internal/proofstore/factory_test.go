package proofstore

import (
	"testing"

	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
)

func TestOpenTiKVBackendIsExplicitlyUnavailable(t *testing.T) {
	t.Parallel()

	if _, err := Open(Config{Kind: BackendTiKV}); trusterr.CodeOf(err) != trusterr.CodeInvalidArgument {
		t.Fatalf("Open(tikv without PD) error code = %s, want %s", trusterr.CodeOf(err), trusterr.CodeInvalidArgument)
	}
	if _, err := Open(Config{Kind: BackendTiKV, TiKVPDAddresses: []string{"127.0.0.1:2379"}}); trusterr.CodeOf(err) != trusterr.CodeFailedPrecondition {
		t.Fatalf("Open(tikv) error code = %s, want %s", trusterr.CodeOf(err), trusterr.CodeFailedPrecondition)
	}
}
