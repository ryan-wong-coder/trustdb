package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestHashFileStream_ProducesCorrectDigest pins the streaming hasher's
// output to a reference sha256 computed via crypto/sha256, making sure
// our progress reporter never mutates bytes on the way to the hash.
func TestHashFileStream_ProducesCorrectDigest(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "payload.bin")
	// Use a non-power-of-two size so io.Copy's last partial buffer
	// reliably exercises the "final chunk" branch of progressReader.
	data := make([]byte, 3*1024*1024+17)
	for i := range data {
		data[i] = byte(i * 31)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	wantSum := sha256.Sum256(data)

	info, err := hashFileStream(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("hashFileStream() error = %v", err)
	}
	if info.Size != int64(len(data)) {
		t.Fatalf("size = %d, want %d", info.Size, len(data))
	}
	if info.ContentHash != hex.EncodeToString(wantSum[:]) {
		t.Fatalf("hash = %s, want %s", info.ContentHash, hex.EncodeToString(wantSum[:]))
	}
	if info.Name != "payload.bin" {
		t.Fatalf("name = %s", info.Name)
	}
}

// TestHashFileStream_EmitsProgressAndFinalTick checks the reporter's
// throttle: we should receive at least one interim tick AND a final
// tick where bytes_hashed == size, regardless of chunk boundaries.
func TestHashFileStream_EmitsProgressAndFinalTick(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "payload.bin")
	// Big enough to cross the 8 MiB chunkThreshold at least twice.
	data := make([]byte, 20*1024*1024)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var ticks atomic.Int64
	var lastRead atomic.Int64
	info, err := hashFileStream(context.Background(), path, func(read, total int64) {
		ticks.Add(1)
		lastRead.Store(read)
		if read > total {
			t.Errorf("read %d overruns total %d", read, total)
		}
	})
	if err != nil {
		t.Fatalf("hashFileStream() error = %v", err)
	}
	if ticks.Load() < 2 {
		t.Fatalf("only %d progress ticks fired, expected >= 2", ticks.Load())
	}
	if lastRead.Load() != info.Size {
		t.Fatalf("final tick saw %d bytes, want %d", lastRead.Load(), info.Size)
	}
}

// TestHashFileStream_Cancellation proves that cancelling the context
// aborts the hash mid-file and surfaces context.Canceled to the
// caller — the guarantee StartHashing relies on to report
// hash:cancelled.
func TestHashFileStream_Cancellation(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "payload.bin")
	// Large enough that the first tick will fire and give us a
	// chance to cancel before EOF.
	data := make([]byte, 40*1024*1024)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var fired atomic.Bool
	done := make(chan error, 1)
	go func() {
		_, err := hashFileStream(ctx, path, func(read, total int64) {
			if !fired.Swap(true) {
				cancel()
			}
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hashFileStream did not return after cancel")
	}
}
