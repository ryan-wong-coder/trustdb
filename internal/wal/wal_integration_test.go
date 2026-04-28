//go:build integration

package wal

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWriterReadAllRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "000000000001.wal")
	w, err := OpenWriter(path, 1)
	if err != nil {
		t.Fatalf("OpenWriter() error = %v", err)
	}
	defer w.Close()

	payloads := [][]byte{[]byte("first"), []byte("second")}
	for _, payload := range payloads {
		if _, _, err := w.Append(context.Background(), payload); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	records, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(records) != len(payloads) {
		t.Fatalf("ReadAll() len = %d, want %d", len(records), len(payloads))
	}
	for i := range records {
		if !bytes.Equal(records[i].Payload, payloads[i]) {
			t.Fatalf("record %d payload = %q, want %q", i, records[i].Payload, payloads[i])
		}
		if records[i].Position.Sequence != uint64(i+1) {
			t.Fatalf("record %d sequence = %d", i, records[i].Position.Sequence)
		}
	}
}

func TestReadAllRejectsCorruptCRC(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "000000000001.wal")
	w, err := OpenWriter(path, 1)
	if err != nil {
		t.Fatalf("OpenWriter() error = %v", err)
	}
	if _, _, err := w.Append(context.Background(), []byte("payload")); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	data[headerSize] ^= 0xff
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := ReadAll(path); err == nil {
		t.Fatal("ReadAll() error = nil, want corruption error")
	}
}

func TestOpenWriterAppendsExistingWAL(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "000000000001.wal")
	w, err := OpenWriter(path, 1)
	if err != nil {
		t.Fatalf("OpenWriter() error = %v", err)
	}
	if _, _, err := w.Append(context.Background(), []byte("first")); err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	w, err = OpenWriter(path, 1)
	if err != nil {
		t.Fatalf("reopen OpenWriter() error = %v", err)
	}
	if _, _, err := w.Append(context.Background(), []byte("second")); err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	records, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records len = %d, want 2", len(records))
	}
	if records[0].Position.Sequence != 1 || records[1].Position.Sequence != 2 {
		t.Fatalf("sequences = %d, %d", records[0].Position.Sequence, records[1].Position.Sequence)
	}
}

func TestInspectAndRepairTruncatedTail(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "000000000001.wal")
	w, err := OpenWriter(path, 1)
	if err != nil {
		t.Fatalf("OpenWriter() error = %v", err)
	}
	if _, _, err := w.Append(context.Background(), []byte("first")); err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	if _, _, err := w.Append(context.Background(), []byte("second")); err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if err := os.Truncate(path, info.Size()-10); err != nil {
		t.Fatalf("Truncate() error = %v", err)
	}
	if _, err := Inspect(path); err == nil {
		t.Fatal("Inspect() error = nil, want truncated tail error")
	}
	repaired, err := Repair(path)
	if err != nil {
		t.Fatalf("Repair() error = %v", err)
	}
	if !repaired.Repaired || repaired.Records != 1 || repaired.TruncatedBytes == 0 {
		t.Fatalf("Repair() = %+v", repaired)
	}
	inspected, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect() after repair error = %v", err)
	}
	if inspected.Records != 1 || inspected.LastSequence != 1 {
		t.Fatalf("Inspect() after repair = %+v", inspected)
	}
}
