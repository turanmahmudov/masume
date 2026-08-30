package db

import "context"

// A driver that speaks one socket refuses a second call while the first one still holds it,
// and returns `conn busy` or `busy buffer` rather than the rows. The screens read on their
// own goroutines, so the calls of one connection wait for each other here.

// CallQueue lets one call at a time onto one connection.
type CallQueue struct {
	slot chan struct{}
}

// NewCallQueue builds a queue with the connection free.
func NewCallQueue() *CallQueue {
	return &CallQueue{slot: make(chan struct{}, 1)}
}

// Take waits for its turn on the connection and returns what gives the turn back. It gives
// up where the context ends first, because an answer nobody waits for is not worth a turn.
func (queue *CallQueue) Take(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case queue.slot <- struct{}{}:
		return queue.giveBack, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// TryTake takes the turn only where the connection is free, and reports whether it did.
func (queue *CallQueue) TryTake() (func(), bool) {
	select {
	case queue.slot <- struct{}{}:
		return queue.giveBack, true
	default:
		return nil, false
	}
}

// giveBack frees the connection for the next call.
func (queue *CallQueue) giveBack() {
	<-queue.slot
}
