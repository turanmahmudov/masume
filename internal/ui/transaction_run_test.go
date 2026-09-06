package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/result"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

type transactionSession struct {
	offlineSession
	state      db.TransactionState
	calls      []string
	beginError error
	readError  error
}

func (session *transactionSession) ReadTransactionState() db.TransactionState {
	return session.state
}

func (session *transactionSession) BeginTransaction(context.Context) error {
	session.calls = append(session.calls, "begin")
	if session.beginError == nil {
		session.state = db.TransactionOpen
	}
	return session.beginError
}

func (session *transactionSession) ReadPage(context.Context, db.ComposedRead, db.ReadWindow) (db.QueryResult, error) {
	session.calls = append(session.calls, "read")
	return db.QueryResult{}, session.readError
}

func TestRunStatementsUsesAutocommit(t *testing.T) {
	for _, test := range []struct {
		name        string
		autocommit  bool
		unsupported bool
		state       db.TransactionState
		beginError  error
		readError   error
		wantCalls   []string
		wantState   db.TransactionState
		wantProblem bool
	}{
		{name: "manual", wantCalls: []string{"begin", "read"}, wantState: db.TransactionOpen},
		{name: "autocommit", autocommit: true, wantCalls: []string{"read"}},
		{name: "unsupported", unsupported: true, wantCalls: []string{"read"}},
		{name: "joined", state: db.TransactionOpen, wantCalls: []string{"read"}, wantState: db.TransactionOpen},
		{name: "autocommit joins", autocommit: true, state: db.TransactionOpen, wantCalls: []string{"read"}, wantState: db.TransactionOpen},
		{name: "begin fails", beginError: errors.New("begin failed"), wantCalls: []string{"begin"}, wantProblem: true},
		{name: "read fails", readError: errors.New("read failed"), wantCalls: []string{"begin", "read"}, wantState: db.TransactionOpen, wantProblem: true},
		{name: "failed transaction", state: db.TransactionFailed, wantState: db.TransactionFailed, wantProblem: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.state == "" {
				test.state = db.TransactionNone
			}
			if test.wantState == "" {
				test.wantState = db.TransactionNone
			}
			session := &transactionSession{
				offlineSession: offlineSession{capabilities: core.Capabilities{HasTransactions: !test.unsupported}},
				state:          test.state, beginError: test.beginError, readError: test.readError,
			}
			answered := runStatements(1, 2, 3, 0, session,
				[]db.ComposedRead{{Text: "select 1", Display: "select 1"}}, 100,
				writeplan.UndoPlan{}, nil, "test", test.autocommit)().(queryRanMsg)
			if !reflect.DeepEqual(session.calls, test.wantCalls) || session.state != test.wantState {
				t.Fatalf("calls %v, state %q; want %v, %q", session.calls, session.state, test.wantCalls, test.wantState)
			}
			if (answered.Problem != "") != test.wantProblem {
				t.Fatalf("problem %q; want failure %v", answered.Problem, test.wantProblem)
			}
		})
	}
}

func TestRunStatementsDoesNotBeginTransactionControlSQL(t *testing.T) {
	for _, sql := range []string{
		"begin", "BEGIN IMMEDIATE", "start transaction", "commit", "rollback", "end", "abort",
		"savepoint saved", "release savepoint saved", "rollback to saved", "rollback work to saved",
		"/* transaction */ COMMIT AND CHAIN", "-- transaction\nROLLBACK",
		"set transaction read only", "set session characteristics as transaction read only",
		"prepare transaction 'saved'", "commit prepared 'saved'",
	} {
		t.Run(sql, func(t *testing.T) {
			session := &transactionSession{offlineSession: offlineSession{
				capabilities: core.Capabilities{HasTransactions: true},
			}}
			answered := runOneStatement(runOneStatementDeps{
				session: session, read: db.ComposedRead{Text: sql}, rowLimit: 100,
			})().(queryRanMsg)
			if answered.Problem != "" || !reflect.DeepEqual(session.calls, []string{"read"}) {
				t.Fatalf("calls %v, problem %q", session.calls, answered.Problem)
			}
		})
	}
}

func openTransactionSession(t *testing.T) db.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transactions.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := engines.CreateAdapters().Open(context.Background(), cfg.Profile{
		Name: "transactions", Engine: core.EngineSqlite, Database: path,
		AccessMode: cfg.AccessWrite, PageSize: 100,
		WritePlan: cfg.PlanOff, ConfirmWrites: cfg.ConfirmOff,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if _, err := session.RunQuery(context.Background(), "create table entries (id integer primary key)", 100, nil); err != nil {
		t.Fatal(err)
	}
	return session
}

func runTransactionStatement(t *testing.T, session db.Session, sql string, autocommit bool) queryRanMsg {
	t.Helper()
	read := session.Composer().ComposeStatementRead(db.BoundText{Text: sql}, core.ReadRewrite{})
	return runOneStatement(runOneStatementDeps{
		session: session, read: read, rowLimit: 100, autocommit: autocommit,
	})().(queryRanMsg)
}

func TestRunStatementsLeavesWritesForExplicitTransactionActions(t *testing.T) {
	for _, autocommit := range []bool{false, true} {
		for _, action := range []ActionID{ActionCommitTransaction, ActionRollbackTransaction} {
			t.Run(string(action)+"/"+map[bool]string{false: "manual", true: "auto"}[autocommit], func(t *testing.T) {
				session := openTransactionSession(t)
				model := buildOfflineModel(t, 100, 30)
				connection := model.Active()
				connection.Session = session
				connection.Autocommit = autocommit
				if autocommit {
					_, command := model.runTransaction(connection, ActionBeginTransaction)
					if answered := command().(transactionRanMsg); answered.Problem != "" {
						t.Fatal(answered.Problem)
					}
				}
				_, command := model.execute(connection, connection.Active(), []string{"insert into entries values (1)"})
				if command == nil {
					t.Fatal("the editor returned no command")
				}
				connection.Autocommit = !autocommit
				if answered := command().(queryRanMsg); answered.Problem != "" {
					t.Fatal(answered.Problem)
				}
				if session.ReadTransactionState() != db.TransactionOpen {
					t.Fatal("the write closed the transaction")
				}
				_, command = model.runTransaction(connection, action)
				if answered := command().(transactionRanMsg); answered.Problem != "" {
					t.Fatal(answered.Problem)
				}
				answered := runTransactionStatement(t, session, "select * from entries", true)
				wantRows := 0
				if action == ActionCommitTransaction {
					wantRows = 1
				}
				if answered.Problem != "" || len(answered.Result.Rows) != wantRows || session.ReadTransactionState() != db.TransactionNone {
					t.Fatalf("rows %v, state %q, problem %q", answered.Result.Rows, session.ReadTransactionState(), answered.Problem)
				}
			})
		}
	}
}

func TestRunStatementsBeginsAgainAfterBatchCommit(t *testing.T) {
	model := buildOfflineModel(t, 100, 30)
	connection := model.Active()
	connection.Session = openTransactionSession(t)
	connection.Autocommit = false
	_, command := model.execute(connection, connection.Active(), []string{
		"insert into entries values (1)", "commit", "insert into entries values (2)",
		"insert into missing values (3)", "insert into entries values (4)",
	})
	for index := 0; command != nil; index++ {
		answered := command().(queryRanMsg)
		if answered.Index != index || (answered.Problem != "") != (index == 3) {
			t.Fatalf("statement %d: %+v", index, answered)
		}
		if answered.Problem != "" {
			_, command = model.readQueryAnswer(answered)
			if command != nil || model.runs.count() != 0 {
				t.Fatal("the failed batch continued")
			}
			break
		}
		command = model.askNextStatement(connection, answered)
	}
	if err := connection.Session.RollbackTransaction(context.Background()); err != nil {
		t.Fatal(err)
	}
	answered := runTransactionStatement(t, connection.Session, "select * from entries", true)
	if answered.Problem != "" || !reflect.DeepEqual(answered.Result.Rows, [][]any{{int64(1)}}) {
		t.Fatalf("rows %v, problem %q", answered.Result.Rows, answered.Problem)
	}
}

func TestRunStatementsPreservesAutocommitWrites(t *testing.T) {
	session := openTransactionSession(t)
	answered := runTransactionStatement(t, session, "insert into entries values (1)", true)
	if answered.Problem != "" || session.ReadTransactionState() != db.TransactionNone {
		t.Fatalf("state %q, problem %q", session.ReadTransactionState(), answered.Problem)
	}
	if err := session.RollbackTransaction(context.Background()); err != nil {
		t.Fatal(err)
	}
	answered = runTransactionStatement(t, session, "select * from entries", true)
	if answered.Problem != "" || len(answered.Result.Rows) != 1 {
		t.Fatalf("rows %v, problem %q", answered.Result.Rows, answered.Problem)
	}
}

func TestRunStatementsExecutesTransactionControlSQL(t *testing.T) {
	session := openTransactionSession(t)
	for _, step := range []struct {
		sql   string
		state db.TransactionState
	}{
		{"begin", db.TransactionOpen},
		{"insert into entries values (1)", db.TransactionOpen},
		{"rollback", db.TransactionNone},
		{"begin", db.TransactionOpen},
		{"insert into entries values (2)", db.TransactionOpen},
		{"commit", db.TransactionNone},
		{"select * from entries", db.TransactionOpen},
	} {
		answered := runTransactionStatement(t, session, step.sql, false)
		if answered.Problem != "" || session.ReadTransactionState() != step.state {
			t.Fatalf("%s: state %q, problem %q", step.sql, session.ReadTransactionState(), answered.Problem)
		}
		if step.sql == "select * from entries" && !reflect.DeepEqual(answered.Result.Rows, [][]any{{int64(2)}}) {
			t.Fatalf("rows %v", answered.Result.Rows)
		}
	}
}

func TestRunStatementsJoinsManualTransactionForUndo(t *testing.T) {
	for _, fail := range []bool{false, true} {
		session := openTransactionSession(t)
		read := "select id from entries"
		if fail {
			read = "select id from missing"
		}
		answered := runOneStatement(runOneStatementDeps{
			session: session, rowLimit: 100,
			read: db.ComposedRead{Text: "insert into entries values (1)"},
			undo: writeplan.UndoPlan{Kept: true, Read: read, Limit: 100},
		})().(queryRanMsg)
		if (answered.Problem != "") != fail || session.ReadTransactionState() == db.TransactionNone {
			t.Fatalf("state %q, problem %q", session.ReadTransactionState(), answered.Problem)
		}
		if err := session.RollbackTransaction(context.Background()); err != nil {
			t.Fatal(err)
		}
		answered = runTransactionStatement(t, session, "select * from entries", true)
		if answered.Problem != "" || len(answered.Result.Rows) != 0 {
			t.Fatalf("rows %v, problem %q", answered.Result.Rows, answered.Problem)
		}
	}
}

func TestRunQueryReadPathsBeginManualTransactions(t *testing.T) {
	for _, autocommit := range []bool{false, true} {
		for _, path := range []string{"table", "page", "count", "plan", "grid", "undo", "export", "chat"} {
			t.Run(path+"/"+map[bool]string{false: "manual", true: "auto"}[autocommit], func(t *testing.T) {
				model := buildOfflineModel(t, 100, 30)
				connection := model.Active()
				connection.Session = openTransactionSession(t)
				connection.Autocommit = autocommit
				tab := connection.Active()
				read := connection.Session.Composer().ComposeStatementRead(db.BoundText{Text: "select * from entries"}, core.ReadRewrite{})
				var command tea.Cmd
				switch path {
				case "table":
					tab.Kind = app.TabTable
					tab.Table = db.TableRef{Schema: "main", Name: "entries", Kind: db.RelationTable}
					_, command = model.runTabRead(connection, tab)
				case "page":
					command = readNextPage(1, 1, 0, 1, connection.Session, read, db.ReadWindow{Limit: 100}, autocommit)
				case "count":
					command = countRows(1, 1, 0, 1, connection.Session, read, autocommit)
				case "plan":
					command = readPlan(1, 1, 1, connection.Session, read.Text, false, autocommit)
				case "grid":
					command = applyChanges(1, 1, connection.Session, []db.Change{{
						Payload: query.BoundStatement{SQL: "insert into entries values (1)"},
					}}, autocommit)
				case "undo":
					command = applyUndo(1, connection.Session, writeplan.Undo{Changes: []db.Change{{
						Payload: query.BoundStatement{SQL: "insert into entries values (1)"},
					}}}, autocommit)
				case "export":
					tab.Results.Start([]string{read.Text}, 100)
					tab.Results.Succeed(0, read, db.QueryResult{})
					exportPath := filepath.Join(t.TempDir(), "entries.json")
					command = model.startExport(connection, tab, exportPath,
						buildExportOverlay(exportPath, result.ExportJSON, true))
				case "chat":
					toolDeps := model.buildChatToolDeps(connection, 1, make(chan app.ChatEvent, 10))
					connection.Autocommit = !autocommit
					command = func() tea.Msg {
						_, err := toolDeps.Runner.RunStatement(context.Background(), "insert into entries values (1)", 100)
						answered := queryRanMsg{}
						if err != nil {
							answered.Problem = err.Error()
						}
						return answered
					}
				}
				answered := command()
				problem := ""
				switch answered := answered.(type) {
				case queryRanMsg:
					problem = answered.Problem
				case pageReadMsg:
					problem = answered.Problem
				case countedMsg:
					problem = answered.Problem
				case planReadMsg:
					problem = answered.Problem
				case changesAppliedMsg:
					problem = answered.Problem
				case undoWrittenMsg:
					problem = answered.Problem
				case exportWrittenMsg:
					problem = answered.Problem
				}
				if problem != "" {
					t.Fatal(problem)
				}
				wantState := db.TransactionOpen
				if autocommit {
					wantState = db.TransactionNone
				}
				if connection.Session.ReadTransactionState() != wantState {
					t.Fatalf("state %q, answer %+v", connection.Session.ReadTransactionState(), answered)
				}
				if err := connection.Session.RollbackTransaction(context.Background()); err != nil {
					t.Fatal(err)
				}
				rows := runTransactionStatement(t, connection.Session, "select * from entries", true)
				wantRows := 0
				if autocommit && (path == "grid" || path == "undo" || path == "chat") {
					wantRows = 1
				}
				if rows.Problem != "" || len(rows.Result.Rows) != wantRows {
					t.Fatalf("rows %v, problem %q", rows.Result.Rows, rows.Problem)
				}
			})
		}
	}
}

func TestRunQueryReadPathsRefuseFailedBegin(t *testing.T) {
	for _, path := range []string{"page", "count", "plan", "grid"} {
		t.Run(path, func(t *testing.T) {
			session := &transactionSession{
				offlineSession: offlineSession{capabilities: core.Capabilities{HasTransactions: true}},
				state:          db.TransactionNone, beginError: errors.New("begin failed"),
			}
			read := db.ComposedRead{Text: "select 1"}
			problem := ""
			switch path {
			case "page":
				problem = readNextPage(1, 1, 0, 1, session, read, db.ReadWindow{Limit: 100}, false)().(pageReadMsg).Problem
			case "count":
				problem = countRows(1, 1, 0, 1, session, read, false)().(countedMsg).Problem
			case "plan":
				problem = readPlan(1, 1, 1, session, read.Text, false, false)().(planReadMsg).Problem
			case "grid":
				problem = applyChanges(1, 1, session, []db.Change{{}}, false)().(changesAppliedMsg).Problem
			}
			if problem == "" || !reflect.DeepEqual(session.calls, []string{"begin"}) {
				t.Fatalf("calls %v, problem %q", session.calls, problem)
			}
		})
	}
}
