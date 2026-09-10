package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/dump"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// A dump is written outside the render loop, a batch of rows at a time. A restore runs the
// statements of the file the same way.

// dumpWrittenMsg carries what the dump wrote, or why it stopped.
type dumpWrittenMsg struct {
	ConnectionID int
	Path         string
	Report       dump.Report
	Problem      string
}

// restoreRanMsg carries what the restore ran, or why it stopped.
type restoreRanMsg struct {
	ConnectionID int
	Report       dump.RunReport
	Problem      string
}

const dumpTransactionProblem = "commit or roll back the open transaction first"

// openDump asks for the file and the settings rather than writing at once. The target is the
// schema or the table the menu was opened on.
func (model *Model) openDump(
	connection *app.Connection, target string, options dump.Options,
) (tea.Model, tea.Cmd) {
	// The file is offered in the directory the client was started in, named after the
	// target and the moment, so two dumps of one schema never collide.
	path := result.BuildExportFilename(target, "sql", time.Now().Format("060102150405"))
	if directory, err := os.Getwd(); err == nil {
		path = filepath.Join(directory, path)
	}
	if options.Content == "" {
		options.Content = dump.ContentAll
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump, Title: " dump ",
		Dump: app.DumpRequest{
			Mode: app.DumpWrite, Stage: app.DumpForm, Path: path,
			Target: target, Options: options,
		},
		Draft: app.NewEditorBuffer(path, len(path)),
	}
	return model, nil
}

// openRestore asks for the file the statements are read from.
func (model *Model) openRestore(connection *app.Connection) (tea.Model, tea.Cmd) {
	if connection.Profile().AccessMode == cfg.AccessReadOnly {
		connection.ShowError("this connection is read-only; a dump cannot be restored through it")
		return model, nil
	}
	if connection.Session.ReadTransactionState() != db.TransactionNone {
		connection.ShowError(dumpTransactionProblem)
		return model, nil
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump, Title: " restore ",
		Dump:  app.DumpRequest{Mode: app.DumpRestore, Stage: app.DumpPick},
		Draft: app.NewEditorBuffer("", 0),
	}
	return model, model.openFilePicker(model.ActiveID(), dump.FileExtensions)
}

// readRestoreFile takes the file the user picked into the form.
func (model *Model) readRestoreFile(connection *app.Connection, path string) {
	overlay := &connection.Overlay
	overlay.Dump.Path = path
	overlay.Dump.Stage = app.DumpForm
	overlay.Notice = ""
	overlay.Field = dumpPathField
	overlay.Draft = app.NewEditorBuffer(path, len(path))
}

// stepDump writes the dump, or runs the file of a restore.
func (model *Model) stepDump(
	connection *app.Connection, overlay *app.Overlay,
) (tea.Model, tea.Cmd) {
	held := &overlay.Dump
	if held.Running || held.Stage == app.DumpPick {
		return model, nil
	}
	if overlay.Draft != nil {
		ReadDumpField(overlay, overlay.Draft.Text)
	}
	if problem := FindDumpProblem(*overlay); problem != "" {
		overlay.Notice = problem
		return model, nil
	}
	overlay.Notice = ""

	path := core.ExpandHomePath(strings.TrimSpace(held.Path))
	if held.Mode == app.DumpRestore {
		return model.startRestore(connection, path)
	}
	// An existing file is never written over without a yes.
	if _, err := os.Stat(path); err == nil {
		options, target := held.Options, held.Target
		connection.Overlay = app.Overlay{
			Kind: app.OverlayConfirm, Title: " overwrite the file ",
			Body: path + " already exists. Overwrite the file?",
			Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
				if !confirmed {
					return nil
				}
				_, command := model.startDump(connection, path, target, options)
				return carryAnswer(command)
			}},
		}
		return model, nil
	}
	return model.startDump(connection, path, held.Target, held.Options)
}

// startDump writes the file outside the render loop.
func (model *Model) startDump(
	connection *app.Connection, path, name string, options dump.Options,
) (tea.Model, tea.Cmd) {
	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump, Title: " dump ",
		Dump: app.DumpRequest{
			Mode: app.DumpWrite, Stage: app.DumpForm, Path: path,
			Target: name, Options: options, Running: true,
		},
		Draft: app.NewEditorBuffer(path, len(path)),
	}
	session := connection.Session
	autocommit := connection.Autocommit
	id := model.ActiveID()
	// Closing the card ends the dump.
	ctx, stop := context.WithCancel(context.Background())
	connection.BeginDump(stop)

	updates, follow := startProgress(id, app.OverlayDump)
	options.OnProgress = func(held dump.Progress) {
		sendProgress(updates, app.Progress{
			Done: int64(held.Tables), Total: int64(held.OfTables), Label: "tables",
			Detail: describeDumpDetail(held),
		})
	}

	return model, tea.Batch(follow, func() tea.Msg {
		defer stop()
		defer closeProgress(updates)
		fail := func(reason string) tea.Msg {
			return dumpWrittenMsg{ConnectionID: id, Path: path, Problem: reason}
		}
		// The reads of a dump are statements of this connection, so an autocommit that is
		// off opens a transaction for them, as it does for a read of the editor.
		if err := beginManualTransaction(ctx, session, autocommit, "select"); err != nil {
			return fail(db.DescribeError(err))
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fail(err.Error())
		}
		// Written beside the file and moved over it at the end, so a read that fails part
		// way leaves the file that was there whole.
		file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.part")
		if err != nil {
			return fail(err.Error())
		}
		temporaryPath := file.Name()
		dropTemporary := func() {
			_ = file.Close()
			_ = os.Remove(temporaryPath)
		}
		// The file holds the rows of the database, so it stays readable by its owner
		// alone, the way an export does.
		if modeErr := file.Chmod(0o600); modeErr != nil {
			dropTemporary()
			return fail(modeErr.Error())
		}

		report, writeErr := dump.Write(ctx, session, options, file)
		if writeErr != nil {
			dropTemporary()
			return fail(db.DescribeError(writeErr))
		}
		if closeErr := file.Close(); closeErr != nil {
			_ = os.Remove(temporaryPath)
			return fail(closeErr.Error())
		}
		if renameErr := os.Rename(temporaryPath, path); renameErr != nil {
			_ = os.Remove(temporaryPath)
			return fail(renameErr.Error())
		}
		return dumpWrittenMsg{ConnectionID: id, Path: path, Report: report}
	})
}

// describeDumpDetail names the table the dump is reading, and the rows it has written.
func describeDumpDetail(held dump.Progress) string {
	written := present.FormatCountOf(held.Rows, "row", "rows")
	if held.Table == "" {
		return written
	}
	return held.Table + " · " + written
}

// startRestore runs the statements of the file outside the render loop.
func (model *Model) startRestore(
	connection *app.Connection, path string,
) (tea.Model, tea.Cmd) {
	if connection.Session.ReadTransactionState() != db.TransactionNone {
		connection.Overlay.Notice = dumpTransactionProblem
		return model, nil
	}
	connection.Overlay.Dump.Running = true
	connection.Overlay.Dump.Path = path

	session := connection.Session
	id := model.ActiveID()
	ctx, stop := context.WithCancel(context.Background())
	connection.BeginDump(stop)

	updates, follow := startProgress(id, app.OverlayDump)
	runner := &restoreRunner{
		session: session, autocommit: connection.Autocommit,
		onStatement: func(count int) {
			sendProgress(updates, app.Progress{Done: int64(count), Label: "statements"})
		},
	}
	connection.Overlay.Dump.Progress = app.Progress{Label: "statements"}

	return model, tea.Batch(follow, func() tea.Msg {
		defer stop()
		defer closeProgress(updates)
		report, err := dump.RunFile(ctx, runner, path, session.Dialect().Syntax)
		if err != nil {
			return restoreRanMsg{
				ConnectionID: id, Report: report,
				Problem: "statement " + present.FormatCount(int64(report.Statements+1)) +
					" failed: " + db.DescribeError(err),
			}
		}
		return restoreRanMsg{ConnectionID: id, Report: report}
	})
}

// restoreRunner runs one statement of a file the way the editor runs one: an autocommit that
// is off opens a transaction first, and the user commits it.
type restoreRunner struct {
	session    db.Session
	autocommit bool
	// onStatement counts the statements that ran, for the card to draw.
	onStatement func(count int)
	ran         int
}

func (runner *restoreRunner) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (db.QueryResult, error) {
	if err := beginManualTransaction(ctx, runner.session, runner.autocommit, sql); err != nil {
		return db.QueryResult{}, err
	}
	answered, err := runner.session.RunQuery(ctx, sql, rowLimit, params)
	if err != nil {
		return answered, err
	}
	runner.ran++
	if runner.onStatement != nil {
		runner.onStatement(runner.ran)
	}
	return answered, nil
}

// readDumpWritten reports what the dump wrote.
func (model *Model) readDumpWritten(answered dumpWrittenMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if connection.Overlay.Kind == app.OverlayDump {
		connection.Overlay.Dump.Running = false
	}
	if answered.Problem != "" {
		if connection.Overlay.Kind == app.OverlayDump {
			connection.Overlay.Notice = answered.Problem
			return model, nil
		}
		connection.ShowError(answered.Problem)
		return model, nil
	}

	connection.Overlay = app.Overlay{}
	connection.Show("dumped " + answered.Report.Describe() + " to " + answered.Path)
	return model, nil
}

// readRestoreRan reports what the restore ran, and reads the object tree again, because the
// file can make and drop tables.
func (model *Model) readRestoreRan(answered restoreRanMsg) (tea.Model, tea.Cmd) {
	connection, id, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if connection.Overlay.Kind == app.OverlayDump {
		connection.Overlay.Dump.Running = false
	}
	ran := present.FormatCountOf(int64(answered.Report.Statements), "statement", "statements")

	if answered.Problem != "" {
		problem := answered.Problem + ", after " + ran
		if connection.Overlay.Kind == app.OverlayDump {
			connection.Overlay.Notice = problem
		} else {
			connection.ShowError(problem)
		}
		return model, readCatalog(id, connection.Session, quietCatalogRead)
	}

	connection.Overlay = app.Overlay{}
	connection.Show("ran " + ran)
	return model, readCatalog(id, connection.Session, quietCatalogRead)
}
