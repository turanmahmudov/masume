package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
)

func TestPlainCatalogAnswersEmptyLists(t *testing.T) {
	catalog := db.PlainCatalog{}
	ctx := context.Background()

	roles, err := catalog.ListRoles(ctx)
	if err != nil || len(roles) != 0 {
		t.Errorf("roles answered %v, %v", roles, err)
	}
	objects, err := catalog.ListSchemaObjects(ctx)
	if err != nil || len(objects) != 0 {
		t.Errorf("objects answered %v, %v", objects, err)
	}
	held, err := catalog.ListRelationships(ctx)
	if err != nil || len(held) != 0 {
		t.Errorf("relationships answered %v, %v", held, err)
	}
}

func TestNoUserTransactionsRefuseToOpen(t *testing.T) {
	keeper := db.NoUserTransactions{}
	if keeper.ReadTransactionState() != db.TransactionNone {
		t.Errorf("the state reads %q, wanted none", keeper.ReadTransactionState())
	}

	ctx := context.Background()
	for _, held := range []struct {
		name string
		call func() error
	}{
		{"begin", func() error { return keeper.BeginTransaction(ctx) }},
		{"commit", func() error { return keeper.CommitTransaction(ctx) }},
		{"rollback", func() error { return keeper.RollbackTransaction(ctx) }},
	} {
		err := held.call()
		if err == nil {
			t.Errorf("%s answered no error", held.name)
			continue
		}
		if !errors.Is(err, db.ErrDatabase) {
			t.Errorf("%s does not read as a database error: %v", held.name, err)
		}
		if described := db.DescribeError(err); described !=
			"this server does not hold a transaction the user drives" {
			t.Errorf("%s describes as %q", held.name, described)
		}
	}
}

func TestNoServerSessionsRefuseEveryCall(t *testing.T) {
	admin := db.NoServerSessions{}
	ctx := context.Background()

	for _, held := range []struct {
		name     string
		call     func() error
		describe string
	}{
		{"activity", func() error {
			activity, err := admin.ListActivity(ctx)
			if len(activity) != 0 {
				t.Errorf("activity answered %v", activity)
			}
			return err
		}, "this server does not list its sessions"},
		{"cancel of another session", func() error {
			stopped, err := admin.CancelBackend(ctx, 1, false)
			if stopped {
				t.Error("a refused cancel reported that it stopped one")
			}
			return err
		}, "this server does not stop another session"},
		{"cancel of the running statement", func() error {
			stopped, err := admin.CancelRunningQuery(ctx)
			if stopped {
				t.Error("a refused cancel reported that it stopped one")
			}
			return err
		}, "this server does not cancel a running statement"},
	} {
		err := held.call()
		if err == nil {
			t.Errorf("%s answered no error", held.name)
			continue
		}
		if !errors.Is(err, db.ErrDatabase) {
			t.Errorf("%s does not read as a database error: %v", held.name, err)
		}
		if described := db.DescribeError(err); described != held.describe {
			t.Errorf("%s describes as %q", held.name, described)
		}
	}
}
