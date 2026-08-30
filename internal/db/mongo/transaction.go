package mongo

import (
	"context"
	"errors"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/turanmahmudov/masume/internal/db"
)

// A transaction of the user, held open across several statements. It needs a replica set
// or a sharded cluster: a standalone server has none, and says so rather than pretending.
//
// The server allows only reads and writes of documents inside one. A catalog read, an
// index build and an explain are all refused there, so those calls run outside the
// transaction and never join it.
//
// Any operation the server refuses aborts the transaction, and every later call returns
// that it was aborted. The state is marked failed at that point, so the user is told to
// roll back rather than meeting the same refusal again.

// transactionHolder holds the transaction of one session.
type transactionHolder struct {
	// held is what the frame reads and a statement records.
	held db.TransactionMark
	// queue lets one call at a time onto the session of a transaction, because a server
	// session takes one call at a time and the screens read on their own goroutines.
	queue *db.CallQueue
	// guard holds the swap of the open session.
	guard  sync.Mutex
	opened *mongo.Session
}

func newTransactionHolder() *transactionHolder {
	return &transactionHolder{queue: db.NewCallQueue()}
}

// readOpened returns the session of the open transaction, or nothing where none is open.
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

// holdSession waits for its turn and returns the context every call runs in: the one of
// the open transaction, or the one the caller gave where none is open.
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

// IsServerError is true where the server refused a call, rather than this client refusing
// to send one.
func IsServerError(err error) bool {
	var reported mongo.ServerError
	return errors.As(err, &reported)
}

// noteServerFailure records that the transaction is over where the server refused a call
// inside it. Every operation the server refuses aborts the transaction, and a statement
// this client would not send never reached it.
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
			"hold a transaction: one needs a replica set or a sharded cluster, " +
				"and this server is standalone")
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

// commitAttempts is how many times a commit whose result the server could not report is
// sent again. A commit is safe to repeat: the server applies the work once.
const commitAttempts = 3

// CommitTransaction applies the work of the transaction. The transaction is over whatever
// the server returns, so a commit it refused still leaves the session free.
func (session *mongoSession) CommitTransaction(ctx context.Context) error {
	return session.endTransaction(ctx, func(opened *mongo.Session) error {
		var err error
		for range commitAttempts {
			err = opened.CommitTransaction(ctx)
			// The server could not report whether the work landed, which is a reason
			// to ask again rather than to tell the user it failed.
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

// isUnknownCommitResult is true where the commit may or may not have landed.
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

// changeRun holds the transaction one set of staged changes runs in: the one the user
// opened, or one this run opens for itself.
type changeRun struct {
	session *mongoSession
	// inside is the context every change runs in.
	inside context.Context
	// owned is the transaction this run opened, which it also ends.
	owned *mongo.Session
}

// begin opens a transaction for this run alone, so the whole set lands or none of it does.
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

// ApplyChanges applies the staged work of the grid. It joins the transaction of the user
// where one is open. Where none is, it opens one of its own so the whole set lands or none
// of it does, and a standalone server that holds no transaction applies them in order.
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

// applyEachChange applies the changes in order, for a server that holds no transaction.
// A failure leaves the changes before it applied, and the message says which one failed.
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
		return db.NewDatabaseError("this change names no command")
	}
	return db.WrapDatabaseError(applyErr)
}

// deploymentHoldsTransactions is true where the server the connection reached holds a
// transaction: a replica set names itself, and a router of a sharded cluster says what it
// is. A standalone server returns neither.
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
