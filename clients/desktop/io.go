package main

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ryan-wong-coder/trustdb/internal/cborx"
)

// marshalCBOR keeps a single import path for CBOR output so future
// replacements (e.g. switching to a deterministic encoder) only
// touch one file in the desktop package.
func marshalCBOR(v any) ([]byte, error) {
	return cborx.Marshal(v)
}

// writeFileAtomic writes to a sibling ".tmp" file and renames into
// place, matching the behaviour of the TrustDB server's disk stores
// so an interrupted export never leaves a half-written file the
// user might mistake for a valid proof.
func writeFileAtomic(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
