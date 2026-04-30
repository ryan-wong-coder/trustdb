package wal

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func BenchmarkWALAppendGroup(b *testing.B) {
	w, err := OpenDirWriter(b.TempDir(), Options{
		FsyncMode:           FsyncGroup,
		GroupCommitInterval: time.Hour,
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = w.Close() })
	payload := bytes.Repeat([]byte{1}, 1024)
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := w.Append(ctx, payload); err != nil {
			b.Fatal(err)
		}
	}
}
