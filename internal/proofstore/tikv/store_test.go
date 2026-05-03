package tikv

import (
	"bytes"
	"encoding/base64"
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

func TestOpenWithOptionsRequiresPDEndpoints(t *testing.T) {
	t.Parallel()

	if _, err := OpenWithOptions(Options{}); trusterr.CodeOf(err) != trusterr.CodeInvalidArgument {
		t.Fatalf("OpenWithOptions without endpoints error code = %s, want %s", trusterr.CodeOf(err), trusterr.CodeInvalidArgument)
	}
}

func TestNormalizeNamespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		namespace string
		want      string
	}{
		{name: "empty defaults", namespace: "", want: "default"},
		{name: "trims whitespace", namespace: " tenant-a/log-a ", want: "tenant-a/log-a"},
		{name: "keeps unicode text", namespace: "租户/日志", want: "租户/日志"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeNamespace(tt.namespace); got != tt.want {
				t.Fatalf("NormalizeNamespace(%q) = %q, want %q", tt.namespace, got, tt.want)
			}
		})
	}
}

func TestNamespaceKeyPrefix(t *testing.T) {
	t.Parallel()

	got := namespaceKeyPrefix("tenant-a/log-a")
	wantSuffix := base64.RawURLEncoding.EncodeToString([]byte("tenant-a/log-a")) + "/"
	if !bytes.HasPrefix(got, []byte(namespacePrefix)) {
		t.Fatalf("namespace prefix %q does not start with %q", got, namespacePrefix)
	}
	if !bytes.HasSuffix(got, []byte(wantSuffix)) {
		t.Fatalf("namespace prefix %q does not end with encoded namespace %q", got, wantSuffix)
	}
}
