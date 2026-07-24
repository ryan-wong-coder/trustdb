package main

import (
	"testing"

	"github.com/wowtrust/trustdb/internal/cryptosuite"
	"github.com/wowtrust/trustdb/internal/proofstore"
)

func newBoundTestLocalStore(t testing.TB, root string) proofstore.LocalStore {
	return newBoundTestLocalStoreForSuite(t, root, cryptosuite.INTLV1)
}

func newBoundTestLocalStoreForSuite(t testing.TB, root string, suiteID cryptosuite.ID) proofstore.LocalStore {
	t.Helper()
	store, err := proofstore.OpenLocalStore(root, suiteID, "local-server", "trustdb-global-log", proofstoreNamespaceID("file", root, "", ""))
	if err != nil {
		t.Fatalf("open test local proofstore: %v", err)
	}
	return *store
}

func newBoundTestProofstoreConfig(kind proofstore.Backend, path string) proofstore.Config {
	return proofstore.Config{
		Kind:        kind,
		Path:        path,
		CryptoSuite: cryptosuite.INTLV1,
		NodeID:      "local-server",
		LogID:       "trustdb-global-log",
		NamespaceID: proofstoreNamespaceID(string(kind), path, "", ""),
	}
}
