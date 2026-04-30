package proofstore

import (
	"testing"

	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
)

func TestOpenTiKVBackendRequiresPDEndpoints(t *testing.T) {
	t.Parallel()

	if _, err := Open(Config{Kind: BackendTiKV}); trusterr.CodeOf(err) != trusterr.CodeInvalidArgument {
		t.Fatalf("Open(tikv without PD) error code = %s, want %s", trusterr.CodeOf(err), trusterr.CodeInvalidArgument)
	}
}
