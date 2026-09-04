package db

import "context"

type PlainCatalog struct{}

func (PlainCatalog) ListRoles(context.Context) ([]DbRole, error) { return nil, nil }

func (PlainCatalog) ListSchemaObjects(context.Context) ([]SchemaObject, error) { return nil, nil }

func (PlainCatalog) ListRelationships(context.Context) ([]Relationship, error) { return nil, nil }

type NoUserTransactions struct{}

func (NoUserTransactions) ReadTransactionState() TransactionState { return TransactionNone }

func (NoUserTransactions) BeginTransaction(context.Context) error {
	return NewUnsupportedError("hold a transaction the user drives")
}

func (NoUserTransactions) CommitTransaction(context.Context) error {
	return NewUnsupportedError("hold a transaction the user drives")
}

func (NoUserTransactions) RollbackTransaction(context.Context) error {
	return NewUnsupportedError("hold a transaction the user drives")
}

type NoServerSessions struct {
	NoServerLoad
}

func (NoServerSessions) ListActivity(context.Context) ([]Activity, error) {
	return nil, NewUnsupportedError("list its sessions")
}

func (NoServerSessions) CancelBackend(context.Context, int64, bool) (bool, error) {
	return false, NewUnsupportedError("stop another session")
}

func (NoServerSessions) CancelRunningQuery(context.Context) (bool, error) {
	return false, NewUnsupportedError("cancel a running statement")
}

// NoServerLoad answers for a server that lists its sessions but reports nothing about the
// load it is under. The dashboard leaves out a panel it has no numbers for.
type NoServerLoad struct{}

func (NoServerLoad) ListLockWaits(context.Context) ([]LockWait, error) {
	return nil, NewUnsupportedError("report which sessions wait for a lock")
}

func (NoServerLoad) ReadServerLoad(context.Context) (ServerLoad, error) {
	return ServerLoad{}, NewUnsupportedError("report the load it is under")
}

func (NoServerLoad) ListSlowStatements(context.Context, int) ([]StatementStat, error) {
	return nil, NewUnsupportedError("report the statements it spends its time in")
}
