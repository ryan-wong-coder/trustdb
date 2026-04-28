package stream

import (
	"context"
	"time"
)

type WindowOptions struct {
	MaxItems int
	MaxDelay time.Duration
}

func Window[T any](ctx context.Context, in <-chan T, opts WindowOptions) <-chan []T {
	out := make(chan []T)
	go func() {
		defer close(out)
		maxItems := opts.MaxItems
		if maxItems <= 0 {
			maxItems = 1
		}
		maxDelay := opts.MaxDelay
		if maxDelay <= 0 {
			maxDelay = time.Hour
		}

		batch := make([]T, 0, maxItems)
		timer := time.NewTimer(maxDelay)
		if !timer.Stop() {
			<-timer.C
		}
		timerActive := false
		defer timer.Stop()

		flush := func() bool {
			if len(batch) == 0 {
				return true
			}
			next := make([]T, len(batch))
			copy(next, batch)
			batch = batch[:0]
			if timerActive {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timerActive = false
			}
			select {
			case out <- next:
				return true
			case <-ctx.Done():
				return false
			}
		}

		for {
			var timerC <-chan time.Time
			if timerActive {
				timerC = timer.C
			}
			select {
			case <-ctx.Done():
				return
			case item, ok := <-in:
				if !ok {
					flush()
					return
				}
				if len(batch) == 0 {
					timer.Reset(maxDelay)
					timerActive = true
				}
				batch = append(batch, item)
				if len(batch) >= maxItems && !flush() {
					return
				}
			case <-timerC:
				timerActive = false
				if !flush() {
					return
				}
			}
		}
	}()
	return out
}
