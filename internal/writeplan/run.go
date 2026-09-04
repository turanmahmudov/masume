package writeplan

import (
	"context"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
)

// Running a write with its undo. The rows are read and the write runs inside one
// transaction, and the read holds those rows until it ends, so no other session can change
// them in between and the undo takes them back to what the write found.

// Writer is the part of a connection a write with an undo needs.
type Writer interface {
	db.SessionInfo
	db.QueryRunner
	db.TransactionKeeper
}

// rollbackWait is how long the rollback of a failed write is given. It is one word to a
// server that is already holding the transaction open for this connection.
const rollbackWait = 5 * time.Second

// rollBack ends the transaction this write opened. A write that passed its time limit fails
// with the context already cancelled, and a rollback sent on that context never leaves the
// client, so it runs on a context of its own.
func rollBack(ctx context.Context, session Writer) {
	held, stop := context.WithTimeout(context.WithoutCancel(ctx), rollbackWait)
	defer stop()
	_ = session.RollbackTransaction(held)
}

// RunWithUndo runs the write and reads its undo inside one transaction. A write that keeps
// no undo, and one on a server without transactions, runs as it stands.
//
// The undo is read before the write. Where the read fails, or reaches more rows than the
// undo allows, nothing is written: the user answered a plan that promised an undo.
func RunWithUndo(
	ctx context.Context, session Writer, plan UndoPlan,
	run func(context.Context) (db.QueryResult, error),
) (db.QueryResult, Undo, error) {
	if !plan.Kept || !session.Capabilities().HasTransactions {
		result, err := run(ctx)
		return result, Undo{Table: plan.Table, Reason: describeUnkeptUndo(session, plan)}, err
	}

	// A transaction of the user is joined, not nested, so nothing commits early.
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
			// The write is in a transaction nothing means to end, so it is rolled
			// back rather than left holding its locks.
			rollBack(ctx, session)
			return db.QueryResult{}, Undo{}, err
		}
	}
	return result, undo, nil
}

// endFailedWrite rolls back the transaction this write opened, and leaves one of the user
// for them to end.
func endFailedWrite(ctx context.Context, session Writer, joined bool, err error) error {
	if !joined {
		rollBack(ctx, session)
	}
	return err
}

// describeUnkeptUndo says why a write ran without an undo.
func describeUnkeptUndo(session Writer, plan UndoPlan) string {
	if plan.Kept {
		return "this server holds no transaction, so an undo cannot be read with the write"
	}
	if plan.Reason == "" && !session.Capabilities().HasTransactions {
		return "this server holds no transaction"
	}
	return plan.Reason
}
