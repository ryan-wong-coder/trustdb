package app

import (
	"github.com/wowtrust/trustdb/internal/cryptosuite"
	"github.com/wowtrust/trustdb/internal/wal"
)

func testWALOptions(namespace string) wal.Options {
	return wal.Options{
		CryptoSuite: cryptosuite.INTLV1,
		NodeID:      "test-node",
		LogID:       "test-log",
		NamespaceID: namespace,
	}
}
