package writeplan

import (
	"context"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
)

// Undo capture and the write use one transaction, with row locking where the dialect supports it.

// Writer is the part of a connection a write with an undo needs.
type Writer interface {
	db.SessionInfo
	db.QueryRunner
	db.TransactionKeeper
}

// rollbackWait is the rollback timeout after a failed write.
const rollbackWait = 5 * time.Second

// rollBack uses a separate context because the write context may already be cancelled.
func rollBack(ctx context.Context, session Writer) {
	held, stop := context.WithTimeout(context.WithoutCancel(ctx), rollbackWait)
	defer stop()
	_ = session.RollbackTransaction(held)
}

// RunWithUndo captures original rows and runs the write in one transaction when undo and transactions are available.
//
// A failed or truncated undo query prevents the write.
func RunWithUndo(
	ctx context.Context, session Writer, plan UndoPlan,
	run func(context.Context) (db.QueryResult, error),
) (db.QueryResult, Undo, error) {
	if !plan.Kept || !session.Capabilities().HasTransactions {
		result, err := run(ctx)
		return result, Undo{Table: plan.Table, Reason: describeUnkeptUndo(session, plan)}, err
	}

	// Use an existing transaction without committing it.
	joined := session.ReadTransactionState() == db.TransactionOpen
	if !joined {
		if err := session.BeginTransaction(ctx); err != nil {
			return db.QueryResult{}, Undo{}, err
		}
	}

	undo, err := ReadUndo(ctx, session, plan)
	if err != nil {
		return db.QueryResult{}, Undo{}, endFailedWrite(ctx, session, joined, err)
	}
	result, err := run(ctx)
	if err != nil {
		return db.QueryResult{}, Undo{}, endFailedWrite(ctx, session, joined, err)
	}

	if !joined {
		if err := session.CommitTransaction(ctx); err != nil {
			// Attempt rollback after a failed commit.
			rollBack(ctx, session)
			return db.QueryResult{}, Undo{}, err
		}
	}
	return result, undo, nil
}

// endFailedWrite rolls back only a transaction opened for this write.
func endFailedWrite(ctx context.Context, session Writer, joined bool, err error) error {
	if !joined {
		rollBack(ctx, session)
	}
	return err
}

// describeUnkeptUndo returns the reason undo is unavailable.
func describeUnkeptUndo(session Writer, plan UndoPlan) string {
	if plan.Kept {
		return "this server does not support transactions; undo capture requires a transaction"
	}
	if plan.Reason == "" && !session.Capabilities().HasTransactions {
		return "this server does not support transactions"
	}
	return plan.Reason
}
