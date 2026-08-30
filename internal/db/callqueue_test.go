package db

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallQueueLetsOneCallThroughAtATime(t *testing.T) {
	queue := NewCallQueue()
	inside := atomic.Int32{}
	most := atomic.Int32{}
	group := sync.WaitGroup{}

	for range 8 {
		group.Go(func() {
			giveBack, err := queue.Take(context.Background())
			if err != nil {
				t.Errorf("the turn was refused: %v", err)
				return
			}
			defer giveBack()
			held := inside.Add(1)
			for {
				seen := most.Load()
				if held <= seen || most.CompareAndSwap(seen, held) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			inside.Add(-1)
		})
	}
	group.Wait()

	if most.Load() != 1 {
		t.Errorf("%d calls were on the connection at once, want 1", most.Load())
	}
}

func TestCallQueueGivesUpWhereTheContextEnds(t *testing.T) {
	queue := NewCallQueue()
	held, err := queue.Take(context.Background())
	if err != nil {
		t.Fatalf("the first turn was refused: %v", err)
	}
	defer held()

	ctx, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if _, waitErr := queue.Take(ctx); waitErr == nil {
		t.Fatal("the second turn was given while the connection was busy")
	}
}

func TestCallQueueRefusesATurnTheContextAlreadyEnded(t *testing.T) {
	queue := NewCallQueue()
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if _, err := queue.Take(ctx); err == nil {
		t.Fatal("a turn was given on a context that had ended")
	}
}

func TestCallQueueTakesAFreeConnectionOnly(t *testing.T) {
	queue := NewCallQueue()
	giveBack, free := queue.TryTake()
	if !free {
		t.Fatal("the free connection was reported busy")
	}
	if _, second := queue.TryTake(); second {
		t.Fatal("the busy connection was reported free")
	}
	giveBack()
	if _, third := queue.TryTake(); !third {
		t.Fatal("the connection stayed busy after the turn was given back")
	}
}
