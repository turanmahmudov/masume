package db

import (
	"sync"

	"github.com/turanmahmudov/masume/internal/query/statement"
)

// TransactionMark holds the state of the transaction of one session. The frame reads it on
// the goroutine that draws, and a statement records it on the goroutine that ran, so the
// read and the write are held apart here.
type TransactionMark struct {
	guard sync.RWMutex
	state TransactionState
}

// ReadState returns the state of the transaction.
func (mark *TransactionMark) ReadState() TransactionState {
	mark.guard.RLock()
	defer mark.guard.RUnlock()
	if mark.state == "" {
		return TransactionNone
	}
	return mark.state
}

// WriteState records the state of the transaction.
func (mark *TransactionMark) WriteState(state TransactionState) {
	mark.guard.Lock()
	defer mark.guard.Unlock()
	mark.state = state
}

// ApplyStatementEffect records what a statement the user ran left the transaction as. A
// `begin` or a `commit` written into the editor never reaches BeginTransaction, so without
// this the mark and the server would drift apart, and a staged write would join a
// transaction that is already committed.
func (mark *TransactionMark) ApplyStatementEffect(effect statement.TransactionEffect) {
	switch effect {
	case statement.EffectOpen:
		mark.WriteState(TransactionOpen)
	case statement.EffectEnd:
		mark.WriteState(TransactionNone)
	case statement.EffectNone:
	}
}

// MarkFailed records that the server refuses every later statement of the transaction. It
// does nothing where none is open.
func (mark *TransactionMark) MarkFailed() {
	mark.guard.Lock()
	defer mark.guard.Unlock()
	if mark.state == TransactionOpen {
		mark.state = TransactionFailed
	}
}
