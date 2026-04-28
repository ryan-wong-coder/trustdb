package logx

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

var ErrWriterClosed = errors.New("logx: writer closed")

type AsyncWriter struct {
	out        io.Writer
	queue      chan []byte
	done       chan struct{}
	wg         sync.WaitGroup
	dropOnFull bool

	closeOnce sync.Once
	mu        sync.Mutex
	closed    bool
	dropped   atomic.Uint64

	errMu sync.Mutex
	err   error
}

func NewAsyncWriter(out io.Writer, bufferSize int, dropOnFull bool) *AsyncWriter {
	if bufferSize <= 0 {
		bufferSize = 1024
	}
	w := &AsyncWriter{
		out:        out,
		queue:      make(chan []byte, bufferSize),
		done:       make(chan struct{}),
		dropOnFull: dropOnFull,
	}
	w.wg.Add(1)
	go w.run()
	return w
}

func (w *AsyncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, ErrWriterClosed
	}
	msg := make([]byte, len(p))
	copy(msg, p)

	if w.dropOnFull {
		select {
		case w.queue <- msg:
			return len(p), nil
		case <-w.done:
			return 0, ErrWriterClosed
		default:
			w.dropped.Add(1)
			return len(p), nil
		}
	}

	select {
	case w.queue <- msg:
		return len(p), nil
	case <-w.done:
		return 0, ErrWriterClosed
	}
}

func (w *AsyncWriter) Close() error {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		close(w.done)
		w.mu.Unlock()
		w.wg.Wait()
	})
	return w.lastErr()
}

func (w *AsyncWriter) Dropped() uint64 {
	return w.dropped.Load()
}

func (w *AsyncWriter) run() {
	defer w.wg.Done()
	for {
		select {
		case msg := <-w.queue:
			w.write(msg)
		case <-w.done:
			for {
				select {
				case msg := <-w.queue:
					w.write(msg)
				default:
					return
				}
			}
		}
	}
}

func (w *AsyncWriter) write(p []byte) {
	if _, err := w.out.Write(p); err != nil {
		w.errMu.Lock()
		if w.err == nil {
			w.err = err
		}
		w.errMu.Unlock()
	}
}

func (w *AsyncWriter) lastErr() error {
	w.errMu.Lock()
	defer w.errMu.Unlock()
	return w.err
}
