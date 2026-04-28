package merkle

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/ryan-wong-coder/trustdb/internal/cborx"
	"github.com/ryan-wong-coder/trustdb/internal/model"
)

type Tree struct {
	leafHashes [][]byte
	root       []byte
}

func Build(records []model.ServerRecord) (Tree, error) {
	if len(records) == 0 {
		return Tree{}, errors.New("merkle: cannot build empty tree")
	}
	leaves := make([][]byte, len(records))
	for i := range records {
		leaf, err := HashLeaf(records[i])
		if err != nil {
			return Tree{}, fmt.Errorf("merkle: hash leaf %d: %w", i, err)
		}
		leaves[i] = leaf
	}
	return Tree{
		leafHashes: leaves,
		root:       treeHash(leaves),
	}, nil
}

func HashLeaf(record model.ServerRecord) ([]byte, error) {
	b, err := cborx.Marshal(record)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write([]byte{0})
	h.Write(b)
	return h.Sum(nil), nil
}

func RootFromLeaves(leaves [][]byte) ([]byte, error) {
	if len(leaves) == 0 {
		return nil, errors.New("merkle: empty leaves")
	}
	copied := make([][]byte, len(leaves))
	for i := range leaves {
		if len(leaves[i]) != sha256.Size {
			return nil, fmt.Errorf("merkle: leaf %d has size %d", i, len(leaves[i]))
		}
		copied[i] = append([]byte(nil), leaves[i]...)
	}
	return treeHash(copied), nil
}

func AuditPathFromLeaves(leaves [][]byte, index uint64) ([][]byte, error) {
	if len(leaves) == 0 {
		return nil, errors.New("merkle: empty leaves")
	}
	if index >= uint64(len(leaves)) {
		return nil, fmt.Errorf("merkle: leaf index out of range: %d", index)
	}
	copied := make([][]byte, len(leaves))
	for i := range leaves {
		if len(leaves[i]) != sha256.Size {
			return nil, fmt.Errorf("merkle: leaf %d has size %d", i, len(leaves[i]))
		}
		copied[i] = append([]byte(nil), leaves[i]...)
	}
	path := auditPath(copied, int(index))
	out := make([][]byte, len(path))
	for i := range path {
		out[i] = append([]byte(nil), path[i]...)
	}
	return out, nil
}

func (t Tree) Root() []byte {
	return append([]byte(nil), t.root...)
}

func (t Tree) LeafHash(index int) ([]byte, error) {
	if index < 0 || index >= len(t.leafHashes) {
		return nil, fmt.Errorf("merkle: leaf index out of range: %d", index)
	}
	return append([]byte(nil), t.leafHashes[index]...), nil
}

func (t Tree) Proof(index int) ([][]byte, error) {
	if index < 0 || index >= len(t.leafHashes) {
		return nil, fmt.Errorf("merkle: leaf index out of range: %d", index)
	}
	path := auditPath(t.leafHashes, index)
	out := make([][]byte, len(path))
	for i := range path {
		out[i] = append([]byte(nil), path[i]...)
	}
	return out, nil
}

func Verify(leafHash []byte, index, treeSize uint64, auditPath [][]byte, root []byte) bool {
	if len(leafHash) != sha256.Size || len(root) != sha256.Size || treeSize == 0 || index >= treeSize {
		return false
	}
	pos := 0
	got, ok := rebuild(leafHash, int(index), int(treeSize), auditPath, &pos)
	if !ok || pos != len(auditPath) {
		return false
	}
	return bytes.Equal(got, root)
}

func HashNode(left, right []byte) ([]byte, error) {
	if len(left) != sha256.Size {
		return nil, fmt.Errorf("merkle: left node has size %d", len(left))
	}
	if len(right) != sha256.Size {
		return nil, fmt.Errorf("merkle: right node has size %d", len(right))
	}
	return hashNode(left, right), nil
}

func treeHash(hashes [][]byte) []byte {
	if len(hashes) == 1 {
		return append([]byte(nil), hashes[0]...)
	}
	k := largestPowerOfTwoLessThan(len(hashes))
	return hashNode(treeHash(hashes[:k]), treeHash(hashes[k:]))
}

func auditPath(hashes [][]byte, index int) [][]byte {
	if len(hashes) == 1 {
		return nil
	}
	k := largestPowerOfTwoLessThan(len(hashes))
	if index < k {
		path := auditPath(hashes[:k], index)
		return append(path, treeHash(hashes[k:]))
	}
	path := auditPath(hashes[k:], index-k)
	return append(path, treeHash(hashes[:k]))
}

func rebuild(leafHash []byte, index, treeSize int, path [][]byte, pos *int) ([]byte, bool) {
	if treeSize == 1 {
		return append([]byte(nil), leafHash...), true
	}
	if *pos >= len(path) {
		return nil, false
	}
	k := largestPowerOfTwoLessThan(treeSize)
	if index < k {
		left, ok := rebuild(leafHash, index, k, path, pos)
		if !ok {
			return nil, false
		}
		right := path[*pos]
		*pos = *pos + 1
		if len(right) != sha256.Size {
			return nil, false
		}
		return hashNode(left, right), true
	}
	right, ok := rebuild(leafHash, index-k, treeSize-k, path, pos)
	if !ok {
		return nil, false
	}
	left := path[*pos]
	*pos = *pos + 1
	if len(left) != sha256.Size {
		return nil, false
	}
	return hashNode(left, right), true
}

func hashNode(left, right []byte) []byte {
	h := sha256.New()
	h.Write([]byte{1})
	h.Write(left)
	h.Write(right)
	return h.Sum(nil)
}

func largestPowerOfTwoLessThan(n int) int {
	if n < 2 {
		return 0
	}
	k := 1
	for k<<1 < n {
		k <<= 1
	}
	return k
}
