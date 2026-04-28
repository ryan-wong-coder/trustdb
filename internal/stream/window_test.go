package stream

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestWindowFlushesByMaxItems(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3
	close(in)

	out := Window(ctx, in, WindowOptions{MaxItems: 2, MaxDelay: time.Hour})
	got := [][]int{<-out, <-out}
	if !reflect.DeepEqual(got, [][]int{{1, 2}, {3}}) {
		t.Fatalf("batches = %#v", got)
	}
	if _, ok := <-out; ok {
		t.Fatal("output channel still open")
	}
}

func TestWindowFlushesByDelay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	in := make(chan int, 1)
	out := Window(ctx, in, WindowOptions{MaxItems: 10, MaxDelay: 10 * time.Millisecond})
	in <- 1
	got := <-out
	if !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("batch = %#v", got)
	}
	close(in)
}

func TestWindowStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan int)
	out := Window(ctx, in, WindowOptions{MaxItems: 10, MaxDelay: time.Hour})
	cancel()
	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("output channel still open")
		}
	case <-time.After(time.Second):
		t.Fatal("window did not stop after context cancel")
	}
}
