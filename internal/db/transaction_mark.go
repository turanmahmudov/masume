package db

import (
	"sync"

	"github.com/turanmahmudov/masume/internal/query/statement"
)

// TransactionMark is a session transaction state with synchronized reads and writes.
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

// ApplyStatementEffect updates transaction state after BEGIN, COMMIT, or ROLLBACK statements from the editor.
func (mark *TransactionMark) ApplyStatementEffect(effect statement.TransactionEffect) {
	switch effect {
	case statement.EffectOpen:
		mark.WriteState(TransactionOpen)
	case statement.EffectEnd:
		mark.WriteState(TransactionNone)
	case statement.EffectNone:
	}
}

// MarkFailed changes an open transaction to failed. Other states remain unchanged.
func (mark *TransactionMark) MarkFailed() {
	mark.guard.Lock()
	defer mark.guard.Unlock()
	if mark.state == TransactionOpen {
		mark.state = TransactionFailed
	}
}
