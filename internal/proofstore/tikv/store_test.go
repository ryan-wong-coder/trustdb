package tikv

import (
	"testing"

	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
)

func TestNormalizePDAddresses(t *testing.T) {
	t.Parallel()

	got := NormalizePDAddresses([]string{"127.0.0.1:2379, 127.0.0.2:2379", ""}, "127.0.0.3:2379")
	want := []string{"127.0.0.1:2379", "127.0.0.2:2379", "127.0.0.3:2379"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestOpenWithOptionsFailsUntilNativeStoreExists(t *testing.T) {
	t.Parallel()

	if _, err := OpenWithOptions(Options{}); trusterr.CodeOf(err) != trusterr.CodeInvalidArgument {
		t.Fatalf("OpenWithOptions without endpoints error code = %s, want %s", trusterr.CodeOf(err), trusterr.CodeInvalidArgument)
	}
	if _, err := OpenWithOptions(Options{PDAddressText: "127.0.0.1:2379"}); trusterr.CodeOf(err) != trusterr.CodeFailedPrecondition {
		t.Fatalf("OpenWithOptions error code = %s, want %s", trusterr.CodeOf(err), trusterr.CodeFailedPrecondition)
	}
}
