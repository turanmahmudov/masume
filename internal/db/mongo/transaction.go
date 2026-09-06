package mongo

import (
	"context"
	"errors"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/turanmahmudov/masume/internal/db"
)

// User transactions span statements and require a replica set or sharded cluster.
// Catalog reads, index operations, and query plans run outside user transactions. Server errors mark the transaction as failed.

// transactionHolder holds the transaction of one session.
type transactionHolder struct {
	// The shared transaction state.
	held db.TransactionMark
	// The queue serializes transaction session calls.
	queue *db.CallQueue
	// guard holds the swap of the open session.
	guard  sync.Mutex
	opened *mongo.Session
}

func newTransactionHolder() *transactionHolder {
	return &transactionHolder{queue: db.NewCallQueue()}
}

// readOpened returns the open transaction session, or nil.
func (holder *transactionHolder) readOpened() *mongo.Session {
	holder.guard.Lock()
	defer holder.guard.Unlock()
	return holder.opened
}

// takeOpened returns the open session and clears it, so a transaction is ended once.
func (holder *transactionHolder) takeOpened() *mongo.Session {
	holder.guard.Lock()
	defer holder.guard.Unlock()
	opened := holder.opened
	holder.opened = nil
	return opened
}

// ReadTransactionState returns the state of the transaction of the user.
func (session *mongoSession) ReadTransactionState() db.TransactionState {
	return session.transaction.held.ReadState()
}

// holdSession reserves the session and adds the open transaction to the context, if present.
func (session *mongoSession) holdSession(
	ctx context.Context,
) (context.Context, func(), error) {
	giveBack, err := session.transaction.queue.Take(ctx)
	if err != nil {
		return nil, nil, db.WrapDatabaseError(err)
	}
	opened := session.transaction.readOpened()
	if opened == nil {
		return ctx, giveBack, nil
	}
	return mongo.NewSessionContext(ctx, opened), giveBack, nil
}

// IsServerError is true for a MongoDB server error.
func IsServerError(err error) bool {
	var reported mongo.ServerError
	return errors.As(err, &reported)
}

// noteServerFailure marks an open transaction as failed after a server error.
func (session *mongoSession) noteServerFailure(err error) error {
	if err != nil && IsServerError(err) {
		session.transaction.held.MarkFailed()
	}
	return err
}

// BeginTransaction opens a transaction the user drives.
func (session *mongoSession) BeginTransaction(ctx context.Context) error {
	if !session.holdsTransactions {
		return db.NewUnsupportedError(
			"support transactions: transactions require a replica set or sharded cluster; " +
				"this server is standalone")
	}
	giveBack, waitErr := session.transaction.queue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if session.transaction.readOpened() != nil {
		return db.NewDatabaseError("a transaction is already open")
	}
	started, startErr := session.client.StartSession()
	if startErr != nil {
		return db.WrapDatabaseError(startErr)
	}
	if txErr := started.StartTransaction(); txErr != nil {
		started.EndSession(ctx)
		return db.WrapDatabaseError(txErr)
	}

	session.transaction.guard.Lock()
	session.transaction.opened = started
	session.transaction.guard.Unlock()
	session.transaction.held.WriteState(db.TransactionOpen)
	return nil
}

// commitAttempts is the maximum commit attempts for unknown results. MongoDB commit retries do not repeat transaction writes.
const commitAttempts = 3

// CommitTransaction commits the transaction and releases the session, including after a commit error.
func (session *mongoSession) CommitTransaction(ctx context.Context) error {
	return session.endTransaction(ctx, func(opened *mongo.Session) error {
		var err error
		for range commitAttempts {
			err = opened.CommitTransaction(ctx)
			// Unknown commit results permit another commit attempt.
			if err == nil || !isUnknownCommitResult(err) || ctx.Err() != nil {
				return err
			}
		}
		return err
	})
}

// unknownCommitResult is the label the server puts on a commit whose result it could not
// report, such as one a step-down interrupted.
const unknownCommitResult = "UnknownTransactionCommitResult"

// isUnknownCommitResult is true if the commit outcome is unknown.
func isUnknownCommitResult(err error) bool {
	var labeled mongo.LabeledError
	return errors.As(err, &labeled) && labeled.HasErrorLabel(unknownCommitResult)
}

// RollbackTransaction throws the work of the transaction away.
func (session *mongoSession) RollbackTransaction(ctx context.Context) error {
	return session.endTransaction(ctx, func(opened *mongo.Session) error {
		err := opened.AbortTransaction(ctx)
		// A transaction the server already aborted is already rolled back, which is
		// what was asked for.
		if err != nil && isAlreadyAborted(err) {
			return nil
		}
		return err
	})
}

// endTransaction ends the transaction one way or the other and frees the session.
func (session *mongoSession) endTransaction(
	ctx context.Context, end func(*mongo.Session) error,
) error {
	giveBack, waitErr := session.transaction.queue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	opened := session.transaction.takeOpened()
	if opened == nil {
		return db.NewDatabaseError("no transaction is open")
	}
	err := end(opened)
	opened.EndSession(ctx)
	session.transaction.held.WriteState(db.TransactionNone)
	return db.WrapDatabaseError(err)
}

// noSuchTransaction is the code the server returns where the transaction it is asked
// about is already over.
const noSuchTransaction = 251

// isAlreadyAborted is true where the server had already aborted the transaction before
// the rollback reached it, which every refusal inside one does.
func isAlreadyAborted(err error) bool {
	var reported mongo.ServerError
	return errors.As(err, &reported) && reported.HasErrorCode(noSuchTransaction)
}

// changeRun is the transaction context for a staged batch, with an optional transaction owned by the batch.
type changeRun struct {
	session *mongoSession
	// inside is the context every change runs in.
	inside context.Context
	// owned is the transaction this run opened, which it also ends.
	owned *mongo.Session
}

// begin opens a transaction owned by the staged batch.
func (run *changeRun) begin(ctx context.Context) error {
	started, err := run.session.client.StartSession()
	if err != nil {
		return err
	}
	if txErr := started.StartTransaction(); txErr != nil {
		started.EndSession(ctx)
		return txErr
	}
	run.owned = started
	run.inside = mongo.NewSessionContext(ctx, started)
	return nil
}

func (run *changeRun) commit(ctx context.Context) error {
	defer run.end(ctx)
	return run.owned.CommitTransaction(ctx)
}

func (run *changeRun) rollback(ctx context.Context) error {
	defer run.end(ctx)
	err := run.owned.AbortTransaction(ctx)
	if err != nil && isAlreadyAborted(err) {
		return nil
	}
	return err
}

func (run *changeRun) end(ctx context.Context) {
	if run.owned != nil {
		run.owned.EndSession(ctx)
		run.owned = nil
	}
}

// ApplyChanges uses an existing or new transaction if supported. Standalone servers apply changes in order without a transaction.
func (session *mongoSession) ApplyChanges(ctx context.Context, changes []db.Change) error {
	bound, giveBack, err := session.holdSession(ctx)
	if err != nil {
		return err
	}
	defer giveBack()

	joins := session.transaction.held.ReadState() == db.TransactionOpen
	if !joins && !session.holdsTransactions {
		return session.applyEachChange(bound, changes)
	}

	run := &changeRun{session: session, inside: bound}
	return db.ApplyChangesInTransaction(ctx, changes, db.ChangeApplication{
		JoinsUserTransaction: joins,
		Begin:                run.begin,
		Commit:               run.commit,
		Rollback:             run.rollback,
		Apply: func(_ context.Context, change db.Change) error {
			return session.applyChange(run.inside, change)
		},
		ReportFailed: func(failure error) { _ = session.noteServerFailure(failure) },
	})
}

// applyEachChange applies changes in order without a transaction. An error leaves earlier changes applied and includes the failed change.
func (session *mongoSession) applyEachChange(ctx context.Context, changes []db.Change) error {
	for _, change := range changes {
		if err := session.applyChange(ctx, change); err != nil {
			if len(changes) > 1 {
				return db.WrapDatabaseOperation(change.Description, err)
			}
			return err
		}
	}
	return nil
}

// applyChange runs one staged command.
func (session *mongoSession) applyChange(ctx context.Context, change db.Change) error {
	command, err := ReadWriteCommand(change)
	if err != nil {
		return err
	}
	collection := session.readDatabase(command.Database).Collection(command.Collection)
	var applyErr error
	switch command.Kind {
	case WriteInsert:
		_, applyErr = collection.InsertOne(ctx, command.Document)
	case WriteUpdate:
		_, applyErr = collection.UpdateOne(ctx, command.Filter, command.Document)
	case WriteDelete:
		_, applyErr = collection.DeleteOne(ctx, command.Filter)
	default:
		return db.NewDatabaseError("the change command is missing")
	}
	return db.WrapDatabaseError(applyErr)
}

// deploymentHoldsTransactions detects a replica set or sharded cluster from the server hello response.
func deploymentHoldsTransactions(hello bson.D) bool {
	for _, field := range hello {
		switch field.Key {
		case "setName":
			if named, isText := field.Value.(string); isText && named != "" {
				return true
			}
		case "msg":
			if written, isText := field.Value.(string); isText && written == "isdbgrid" {
				return true
			}
		}
	}
	return false
}
