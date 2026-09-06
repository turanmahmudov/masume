package db

import "context"

type PlainCatalog struct{}

func (PlainCatalog) ListRoles(context.Context) ([]DbRole, error) { return nil, nil }

func (PlainCatalog) ListSchemaObjects(context.Context) ([]SchemaObject, error) { return nil, nil }

func (PlainCatalog) ListRelationships(context.Context) ([]Relationship, error) { return nil, nil }

type NoUserTransactions struct{}

func (NoUserTransactions) ReadTransactionState() TransactionState { return TransactionNone }

func (NoUserTransactions) BeginTransaction(context.Context) error {
	return NewUnsupportedError("support user-managed transactions")
}

func (NoUserTransactions) CommitTransaction(context.Context) error {
	return NewUnsupportedError("support user-managed transactions")
}

func (NoUserTransactions) RollbackTransaction(context.Context) error {
	return NewUnsupportedError("support user-managed transactions")
}

type NoServerSessions struct {
	NoServerLoad
}

func (NoServerSessions) ListActivity(context.Context) ([]Activity, error) {
	return nil, NewUnsupportedError("list sessions")
}

func (NoServerSessions) CancelBackend(context.Context, int64, bool) (bool, error) {
	return false, NewUnsupportedError("stop another session")
}

func (NoServerSessions) CancelRunningQuery(context.Context) (bool, error) {
	return false, NewUnsupportedError("cancel a running statement")
}

// NoServerLoad is the fallback for engines without server load statistics.
type NoServerLoad struct{}

func (NoServerLoad) ListLockWaits(context.Context) ([]LockWait, error) {
	return nil, NewUnsupportedError("report lock waits")
}

func (NoServerLoad) ReadServerLoad(context.Context) (ServerLoad, error) {
	return ServerLoad{}, NewUnsupportedError("report server load")
}

func (NoServerLoad) ListSlowStatements(context.Context, int) ([]StatementStat, error) {
	return nil, NewUnsupportedError("report slow statements")
}
