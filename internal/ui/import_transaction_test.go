package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/load"
	"github.com/turanmahmudov/masume/internal/query"
)

func TestOpenImportRefusesExistingTransactions(t *testing.T) {
	for _, state := range []db.TransactionState{db.TransactionOpen, db.TransactionFailed} {
		t.Run(string(state), func(t *testing.T) {
			model := buildOfflineModel(t, 100, 30)
			connection := model.Active()
			connection.Session = &transactionSession{state: state}
			_, command := model.openImport(connection, db.TableRef{Name: "entries"}, false)
			if command != nil || connection.Overlay.Kind == app.OverlayImport || connection.Notice == nil || connection.Notice.Text != importTransactionProblem {
				t.Fatal("the import did not refuse the transaction")
			}
			connection.Overlay = app.Overlay{
				Kind: app.OverlayImport, Import: app.ImportRequest{Stage: app.ImportReview},
			}
			_, command = model.stepImport(connection, &connection.Overlay)
			if command != nil || connection.Overlay.Import.Running || connection.Overlay.Notice != importTransactionProblem {
				t.Fatal("the review did not refuse the transaction")
			}
			answered := runImport(connection, 1, load.Plan{}, connection.Session.Dialect(), 0, nil)().(importRanMsg)
			if answered.Problem != importTransactionProblem || connection.Session.ReadTransactionState() != state {
				t.Fatalf("state %q, answer %+v", connection.Session.ReadTransactionState(), answered)
			}
		})
	}
}

func TestRunImportRefusesTransactionOpenedAfterDispatch(t *testing.T) {
	session := openTransactionSession(t)
	connection := app.NewConnection(session, nil, false)
	command := runImport(connection, 1, load.Plan{}, session.Dialect(), 0, nil)
	if answered := runTransactionStatement(t, session, "insert into entries values (1)", false); answered.Problem != "" {
		t.Fatal(answered.Problem)
	}
	answered := command().(importRanMsg)
	if answered.Problem != importTransactionProblem || answered.Written != 0 || session.ReadTransactionState() != db.TransactionOpen {
		t.Fatalf("state %q, answer %+v", session.ReadTransactionState(), answered)
	}
	if err := session.RollbackTransaction(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows := runTransactionStatement(t, session, "select * from entries", true)
	if rows.Problem != "" || len(rows.Result.Rows) != 0 {
		t.Fatalf("the import committed unrelated rows: %+v", rows)
	}
}

func TestRunImportCommitsOrRollsBackItsOwnTransaction(t *testing.T) {
	for _, fail := range []bool{false, true} {
		session := openTransactionSession(t)
		connection := app.NewConnection(session, nil, false)
		path := filepath.Join(t.TempDir(), "entries.csv")
		if err := os.WriteFile(path, []byte("id\n1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		options := load.DefaultReadOptions()
		sample, err := load.ReadSample(path, options)
		if err != nil {
			t.Fatal(err)
		}
		plan := load.BuildPlan(path, options, sample, query.QualifiedName{Schema: "main", Name: "entries"},
			[]load.TargetColumn{{Name: "id", DataType: "integer"}})
		if fail {
			plan.Table.Name = "missing"
		}
		answered := runImport(connection, 1, plan, session.Dialect(), 0, nil)().(importRanMsg)
		if (answered.Problem != "") != fail || session.ReadTransactionState() != db.TransactionNone {
			t.Fatalf("state %q, answer %+v", session.ReadTransactionState(), answered)
		}
		wantRows := 1
		if fail {
			wantRows = 0
		}
		rows := runTransactionStatement(t, session, "select * from entries", true)
		if rows.Problem != "" || len(rows.Result.Rows) != wantRows || answered.Written != wantRows {
			t.Fatalf("rows %+v, import %+v", rows, answered)
		}
	}
}
