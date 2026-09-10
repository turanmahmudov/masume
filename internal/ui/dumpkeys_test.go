package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// The dump and the restore are reached with the keys a user presses: the object menu of the
// tree, the row of the menu, and the key of the card.

// buildTreeDumpModel answers a model whose tree holds one schema and one table, and whose
// connection answers the reads of a dump.
func buildTreeDumpModel(t *testing.T) (*Model, *app.Connection, *dumpSession) {
	t.Helper()
	model, connection, session := buildDumpModel(t)
	session.offlineSession.capabilities = core.Capabilities{WritesDDL: true}
	connection.Catalog.Tables = []db.TableRef{
		{Schema: "public", Name: "orders", Kind: db.RelationTable},
	}
	connection.Catalog.Loading = false
	connection.Active().Focus = app.PaneSidebar
	model.render()

	// The tables of a schema are drawn once the schema is unfolded.
	standOnNode(t, model, connection, present.NodeSchema)
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	model.render()
	return model, connection, session
}

// standOnNode moves the tree cursor to the first row of that kind.
func standOnNode(t *testing.T, model *Model, connection *app.Connection, kind present.TreeNodeKind) {
	t.Helper()
	for at, row := range model.treeRows(connection) {
		if row.Node.Kind == kind {
			connection.Tree.Cursor = at
			return
		}
	}
	t.Fatalf("the tree holds no %s row", kind)
}

// pressKey presses one key on the model.
func pressKey(t *testing.T, model *Model, key tea.KeyPressMsg) tea.Cmd {
	t.Helper()
	held, command := model.Update(key)
	if held != model {
		t.Fatal("a press replaced the model")
	}
	return command
}

// chooseMenuRow moves the cursor of the menu to the action and presses Enter.
func chooseMenuRow(t *testing.T, model *Model, connection *app.Connection, id string) tea.Cmd {
	t.Helper()
	for at, action := range connection.Overlay.Actions {
		if action.ID == id {
			connection.Overlay.List.Cursor = at
			return pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
		}
	}
	t.Fatalf("the menu offers no %s row", id)
	return nil
}

// openObjectMenu presses `m` on the tree row of that kind.
func openObjectMenu(
	t *testing.T, model *Model, connection *app.Connection, kind present.TreeNodeKind,
) {
	t.Helper()
	standOnNode(t, model, connection, kind)
	pressKey(t, model, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if connection.Overlay.Kind != app.OverlayObjectMenu {
		t.Fatalf("`m` on the tree opened %q", connection.Overlay.Kind)
	}
}

func TestTheSchemaMenuWritesADumpFile(t *testing.T) {
	model, connection, _ := buildTreeDumpModel(t)
	openObjectMenu(t, model, connection, present.NodeSchema)
	chooseMenuRow(t, model, connection, app.ObjectDumpSchema)

	if connection.Overlay.Kind != app.OverlayDump {
		t.Fatalf("the menu row opened %q", connection.Overlay.Kind)
	}
	path := filepath.Join(t.TempDir(), "public.sql")
	connection.Overlay.Dump.Path = path
	connection.Overlay.Draft = app.NewEditorBuffer(path, len(path))

	command := pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("Enter on the dump card started nothing")
	}
	answered := findMessage[dumpWrittenMsg](t, command)
	if answered.Problem != "" {
		t.Fatalf("the card answered %+v", answered)
	}

	if held, _ := model.Update(answered); held != model {
		t.Fatal("the answer replaced the model")
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "insert into") {
		t.Errorf("the file holds no row:\n%s", written)
	}
	if connection.Overlay.IsOpen() {
		t.Errorf("the card is still open as %q", connection.Overlay.Kind)
	}
	if connection.Notice == nil || !strings.Contains(connection.Notice.Text, "dumped 1 table") {
		t.Errorf("notice: %+v, want the count of the dump", connection.Notice)
	}
}

func TestTheTableMenuDumpsThatTableAlone(t *testing.T) {
	model, connection, _ := buildTreeDumpModel(t)
	openObjectMenu(t, model, connection, present.NodeTable)
	chooseMenuRow(t, model, connection, app.ObjectDumpTable)

	held := connection.Overlay
	if held.Kind != app.OverlayDump {
		t.Fatalf("the menu row opened %q", held.Kind)
	}
	if len(held.Dump.Options.Tables) != 1 ||
		held.Dump.Options.Tables[0].Name != "orders" {
		t.Fatalf("tables: %v, want the table of the row", held.Dump.Options.Tables)
	}
	if held.Dump.Target != "orders" {
		t.Errorf("target: %q, want the table name", held.Dump.Target)
	}
}

func TestTheSchemaMenuRestoresAFile(t *testing.T) {
	model, connection, session := buildTreeDumpModel(t)
	path := filepath.Join(t.TempDir(), "shop.sql")
	if err := os.WriteFile(path,
		[]byte("create table t (id int);\ninsert into t values (1);\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	openObjectMenu(t, model, connection, present.NodeSchema)
	chooseMenuRow(t, model, connection, app.ObjectRestoreFile)
	if connection.Overlay.Kind != app.OverlayDump ||
		connection.Overlay.Dump.Stage != app.DumpPick {
		t.Fatalf("the menu row opened %q at %q", connection.Overlay.Kind,
			connection.Overlay.Dump.Stage)
	}

	// The picker answers with the file the user chose.
	model.readRestoreFile(connection, path)
	command := pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("Enter on the restore card started nothing")
	}
	answered := findMessage[restoreRanMsg](t, command)
	if answered.Problem != "" {
		t.Fatalf("the card answered %+v", answered)
	}
	if len(session.ran) != 2 {
		t.Errorf("ran: %q, want both statements", session.ran)
	}

	if held, _ := model.Update(answered); held != model {
		t.Fatal("the answer replaced the model")
	}
	if connection.Notice == nil || !strings.Contains(connection.Notice.Text, "ran 2 statements") {
		t.Errorf("notice: %+v, want the count of the run", connection.Notice)
	}
}

func TestEscapeStopsTheDumpThatRuns(t *testing.T) {
	model, connection, _ := buildTreeDumpModel(t)
	stopped := false
	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump,
		Dump: app.DumpRequest{Mode: app.DumpWrite, Stage: app.DumpForm, Running: true},
	}
	connection.BeginDump(func() { stopped = true })

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !stopped {
		t.Error("Escape left the dump running")
	}
	if connection.Overlay.IsOpen() {
		t.Errorf("the card is still open as %q", connection.Overlay.Kind)
	}
}

func TestARestoreIsRefusedOnAReadOnlyConnection(t *testing.T) {
	model, connection, _ := buildTreeDumpModel(t)
	connection.Session.(*dumpSession).offlineSession.profile = cfg.Profile{
		Name: "offline", Engine: "postgres", AccessMode: cfg.AccessReadOnly,
	}

	model.openRestore(connection)
	if connection.Overlay.IsOpen() {
		t.Fatalf("a read-only connection opened %q", connection.Overlay.Kind)
	}
	if connection.Notice == nil || !strings.Contains(connection.Notice.Text, "read-only") {
		t.Fatalf("notice: %+v, want the refusal", connection.Notice)
	}
}

func TestARestoreIsRefusedWhileATransactionIsOpen(t *testing.T) {
	model, connection, session := buildTreeDumpModel(t)
	session.transaction = db.TransactionOpen

	model.openRestore(connection)
	if connection.Overlay.IsOpen() {
		t.Fatalf("an open transaction opened %q", connection.Overlay.Kind)
	}
	if connection.Notice == nil ||
		!strings.Contains(connection.Notice.Text, "transaction") {
		t.Fatalf("notice: %+v, want the refusal", connection.Notice)
	}
}
