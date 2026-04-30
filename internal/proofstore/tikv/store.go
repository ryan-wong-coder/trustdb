// Package tikv documents the planned TiKV proofstore backend boundary.
package tikv

import (
	"strings"

	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
)

// Options configures a future TiKV proofstore. PDAddressText accepts the same
// comma-separated endpoint format used by many TiKV tools and is treated as a
// fallback when PDAddresses is empty.
type Options struct {
	PDAddresses                  []string
	PDAddressText                string
	Keyspace                     string
	RecordIndexMode              string
	ArtifactSyncMode             string
	IndexStorageTokens           bool
	IndexStorageTokensConfigured bool
}

type Store struct{}

// OpenWithOptions intentionally fails until the package has a native,
// persistent proofstore.Store implementation. Returning a local cache-backed
// store here would make metastore=tikv appear durable while silently losing
// data across process restarts.
func OpenWithOptions(opts Options) (*Store, error) {
	if len(NormalizePDAddresses(opts.PDAddresses, opts.PDAddressText)) == 0 {
		return nil, trusterr.New(trusterr.CodeInvalidArgument, "tikv proofstore requires at least one PD endpoint")
	}
	return nil, trusterr.New(trusterr.CodeFailedPrecondition, "tikv proofstore backend is not implemented yet")
}

func NormalizePDAddresses(values []string, text string) []string {
	out := make([]string, 0, len(values)+1)
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	for _, part := range strings.Split(text, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
