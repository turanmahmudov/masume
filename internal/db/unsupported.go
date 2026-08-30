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

type NoServerSessions struct{}

func (NoServerSessions) ListActivity(context.Context) ([]Activity, error) {
	return nil, NewUnsupportedError("list its sessions")
}

func (NoServerSessions) CancelBackend(context.Context, int64, bool) (bool, error) {
	return false, NewUnsupportedError("stop another session")
}

func (NoServerSessions) CancelRunningQuery(context.Context) (bool, error) {
	return false, NewUnsupportedError("cancel a running statement")
}
