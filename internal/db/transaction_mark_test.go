package db

import (
	"sync"
	"testing"
)

func TestTransactionMarkStartsWithNoneOpen(t *testing.T) {
	mark := TransactionMark{}
	if state := mark.ReadState(); state != TransactionNone {
		t.Errorf("a fresh mark reads %q, want %q", state, TransactionNone)
	}
}

func TestTransactionMarkFailsOnlyAnOpenTransaction(t *testing.T) {
	cases := []struct {
		name  string
		from  TransactionState
		after TransactionState
	}{
		{"none stays none", TransactionNone, TransactionNone},
		{"open becomes failed", TransactionOpen, TransactionFailed},
		{"failed stays failed", TransactionFailed, TransactionFailed},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			mark := TransactionMark{}
			mark.WriteState(held.from)
			mark.MarkFailed()
			if state := mark.ReadState(); state != held.after {
				t.Errorf("%q became %q, want %q", held.from, state, held.after)
			}
		})
	}
}

func TestTransactionMarkHoldsTheReadAndTheWriteApart(t *testing.T) {
	mark := TransactionMark{}
	mark.WriteState(TransactionOpen)

	group := sync.WaitGroup{}
	for range 8 {
		group.Go(func() {
			for range 200 {
				mark.MarkFailed()
			}
		})
		group.Go(func() {
			for range 200 {
				if state := mark.ReadState(); state != TransactionOpen &&
					state != TransactionFailed {
					t.Errorf("the mark read %q", state)
					return
				}
			}
		})
	}
	group.Wait()

	if state := mark.ReadState(); state != TransactionFailed {
		t.Errorf("the mark ended as %q, want %q", state, TransactionFailed)
	}
}
