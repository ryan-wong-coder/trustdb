package logx

import (
	"bytes"
	"errors"
	"sync"
	"testing"
)

func TestAsyncWriterFlushesOnClose(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := NewAsyncWriter(&out, 8, false)
	if _, err := w.Write([]byte("a")); err != nil {
		t.Fatalf("Write(a) error = %v", err)
	}
	if _, err := w.Write([]byte("b")); err != nil {
		t.Fatalf("Write(b) error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := out.String(); got != "ab" {
		t.Fatalf("output = %q, want ab", got)
	}
	if _, err := w.Write([]byte("c")); !errors.Is(err, ErrWriterClosed) {
		t.Fatalf("Write after close error = %v, want ErrWriterClosed", err)
	}
}

func TestAsyncWriterDropsWhenFull(t *testing.T) {
	t.Parallel()

	out := &blockingWriter{release: make(chan struct{})}
	w := NewAsyncWriter(out, 1, true)
	if _, err := w.Write([]byte("first")); err != nil {
		t.Fatalf("Write(first) error = %v", err)
	}
	if _, err := w.Write([]byte("second")); err != nil {
		t.Fatalf("Write(second) error = %v", err)
	}
	if _, err := w.Write([]byte("third")); err != nil {
		t.Fatalf("Write(third) error = %v", err)
	}
	if got := w.Dropped(); got == 0 {
		t.Fatal("Dropped() = 0, want at least one dropped log")
	}
	close(out.release)
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

type blockingWriter struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	release chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.release
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}
