package wal

import (
	"bytes"
	"testing"
)

func FuzzReadOne(f *testing.F) {
	encoded, _ := encodeRecord(1, 1, 123, [32]byte{}, []byte("seed"))
	f.Add(encoded)
	f.Add([]byte{0x54, 0x44, 0x57, 0x31})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = readOne(bytes.NewReader(data), 0, [32]byte{})
	})
}
