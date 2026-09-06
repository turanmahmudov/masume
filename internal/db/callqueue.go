package db

import "context"

// Drivers can reject concurrent socket operations with `conn busy` or `busy buffer`. The queue serializes connection calls.

// CallQueue lets one call at a time onto one connection.
type CallQueue struct {
	slot chan struct{}
}

// NewCallQueue builds a queue with the connection free.
func NewCallQueue() *CallQueue {
	return &CallQueue{slot: make(chan struct{}, 1)}
}

// Take waits for the connection and returns a release callback. Context cancellation stops the wait.
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

// TryTake reserves a free connection without waiting and returns a release callback.
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
