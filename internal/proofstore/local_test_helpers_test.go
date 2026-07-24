package proofstore

import (
	"github.com/wowtrust/trustdb/internal/cryptosuite"
	"github.com/wowtrust/trustdb/internal/proofstoremeta"
)

func testLocalStore(root string) LocalStore {
	binding, err := proofstoremeta.New(cryptosuite.INTLV1, "test-node", "test-log", "test-local")
	if err != nil {
		panic(err)
	}
	return *newBoundLocalStore(root, binding)
}
