package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
)

// openSecondConnection adds another connection, as a second pick from the picker would. Its
// tabs are numbered from one, the same as those of the first.
func openSecondConnection(t *testing.T, model *Model) *app.Connection {
	t.Helper()
	held := app.NewConnection(&offlineSession{
		profile:      cfg.Profile{Name: "second", Engine: "postgres"},
		capabilities: model.Active().Session.Capabilities(),
	}, nil, true)
	model.connections.open(held)
	return held
}

// succeedOneRow puts a result of one row into the tab, whose only value names the server it
// came from.
func succeedOneRow(tab *app.Tab, who string) {
	tab.Results.Start([]string{"select " + who}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select " + who, Pageable: true},
		db.QueryResult{
			Columns: []db.ResultColumn{{Name: "who", DataType: "text"}},
			Rows:    [][]any{{who}},
		})
}

// A tab is numbered inside its own connection, so the first tab of every connection is tab one.
// What is kept for one of them must never be read for another: the grid would draw the rows of
// a server the reader is not looking at.
func TestTwoConnectionsDoNotShareWhatIsKeptPerTab(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	first := model.Active()
	second := openSecondConnection(t, model)

	firstTab, secondTab := first.Active(), second.Active()
	if firstTab.ID != secondTab.ID {
		t.Fatalf("the tabs are numbered %d and %d, so nothing would collide",
			firstTab.ID, secondTab.ID)
	}
	succeedOneRow(firstTab, "first")
	succeedOneRow(secondTab, "second")

	firstShape := model.buildGridShape(first, firstTab)
	secondShape := model.buildGridShape(second, secondTab)

	if len(firstShape.Text) != 1 || len(secondShape.Text) != 1 {
		t.Fatalf("the grids drew %d and %d rows, wanted one each",
			len(firstShape.Text), len(secondShape.Text))
	}
	if held := firstShape.Text[0][0]; held != "first" {
		t.Errorf("the first connection drew %q", held)
	}
	if held := secondShape.Text[0][0]; held != "second" {
		t.Errorf("the second connection drew %q, and the first drew %q",
			held, firstShape.Text[0][0])
	}

	// The order the grids are read in must not decide what they draw.
	if held := model.buildGridShape(first, firstTab).Text[0][0]; held != "first" {
		t.Errorf("read again, the first connection drew %q", held)
	}
}

// The head of the result is kept per tab as well, so the labels and the masking of one
// connection never answer for another.
func TestTwoConnectionsDoNotShareTheHeadOfTheResult(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	first := model.Active()
	second := openSecondConnection(t, model)

	firstTab, secondTab := first.Active(), second.Active()
	firstTab.Results.Start([]string{"select id from orders"}, 200)
	firstTab.Results.Succeed(0, db.ComposedRead{Text: "select id from orders"},
		db.QueryResult{
			Columns: []db.ResultColumn{{Name: "id", DataType: "integer"}},
			Rows:    [][]any{{int64(1)}},
		})
	secondTab.Results.Start([]string{"select password from users"}, 200)
	secondTab.Results.Succeed(0, db.ComposedRead{Text: "select password from users"},
		db.QueryResult{
			Columns: []db.ResultColumn{{Name: "password", DataType: "text"}},
			Rows:    [][]any{{"secret"}},
		})

	firstHead := model.resolveGridHead(model.buildTabKey(first, firstTab), first, firstTab,
		firstTab.Results.Active())
	secondHead := model.resolveGridHead(model.buildTabKey(second, secondTab), second, secondTab,
		secondTab.Results.Active())

	if firstHead.labels[0] != "id" {
		t.Errorf("the first connection names its column %q", firstHead.labels[0])
	}
	if secondHead.labels[0] != "password" {
		t.Errorf("the second connection names its column %q", secondHead.labels[0])
	}
	// A column that suggests a secret is masked. The first result holds none, so a shared
	// head would show the second one unmasked.
	if !secondHead.masked[0] {
		t.Error("the password column of the second connection is not masked")
	}
	if firstHead.masked[0] {
		t.Error("the id column of the first connection is masked")
	}
}

// The faults of a buffer are kept per tab too, so a statement in one connection is never
// reported against the catalog of another.
func TestTwoConnectionsDoNotShareTheFaultsOfTheBuffer(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	first := model.Active()
	second := openSecondConnection(t, model)

	firstKey := model.buildTabKey(first, first.Active())
	secondKey := model.buildTabKey(second, second.Active())
	if firstKey == secondKey {
		t.Fatal("the two tabs are named by the same key")
	}
}

// The conversation is kept for the panel on screen. Two connections that hold the same turns
// would draw the same rows, so this proves the key tells them apart rather than relying on the
// rows agreeing by chance.
func TestTwoConnectionsDoNotShareTheConversation(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	first := model.Active()
	second := openSecondConnection(t, model)

	said := []app.ChatMessage{{Role: hist.ChatRoleUser, Content: "which tables are there?"}}
	first.Chat.Messages = said
	second.Chat.Messages = said

	if model.buildChatRowsKey(first, first.Chat, 60) ==
		model.buildChatRowsKey(second, second.Chat, 60) {
		t.Error("the same conversation on two connections is named by one key")
	}
}

// What was kept for a tab goes when the tab goes. One entry holds a whole page of rows as
// text, so a session that opens and closes many tabs would hold every page it ever drew.
func TestWhatWasKeptForATabGoesWithTheTab(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()

	// A connection keeps one tab open whatever is closed, so a second one is opened to
	// have one to close.
	opened := connection.OpenQueryTab("select * from orders")
	kept := connection.OpenQueryTab("select * from customers")
	succeedOneRow(opened, "first")
	succeedOneRow(kept, "second")
	model.buildGridShape(connection, opened)
	model.buildGridShape(connection, kept)

	key := model.buildTabKey(connection, opened)
	if _, held := model.caches.readText(key); !held {
		t.Fatal("nothing was kept for the tab that was drawn")
	}

	connection.CloseTab(connection.IndexOfTab(opened.ID))
	if connection.IndexOfTab(opened.ID) >= 0 {
		t.Fatal("the tab did not close")
	}
	model.render()

	if _, held := model.caches.readText(model.buildTabKey(connection, kept)); !held {
		t.Error("the rows of the tab that stayed open went with the one that closed")
	}

	if _, held := model.caches.readText(key); held {
		t.Error("the rows of the tab were kept after it closed")
	}
	if _, held := model.caches.readHead(key); held {
		t.Error("the head of the tab was kept after it closed")
	}
}

// What was kept for a connection goes when the connection goes.
func TestWhatWasKeptForAConnectionGoesWithIt(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	second := openSecondConnection(t, model)
	secondTab := second.Active()
	succeedOneRow(secondTab, "second")
	model.buildGridShape(second, secondTab)

	key := model.buildTabKey(second, secondTab)
	if _, held := model.caches.readText(key); !held {
		t.Fatal("nothing was kept for the tab that was drawn")
	}

	model.connections.closeActive()
	model.render()

	if _, held := model.caches.readText(key); held {
		t.Error("the rows of the connection were kept after it closed")
	}
}

// The frame keeps what the tabs on screen need, or every frame would draw them again.
func TestWhatTheOpenTabsNeedIsKept(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	succeedOneRow(tab, "first")
	model.buildGridShape(connection, tab)

	key := model.buildTabKey(connection, tab)
	for at := range 5 {
		model.render()
		if _, held := model.caches.readText(key); !held {
			t.Fatalf("frame %d dropped the rows of the tab on screen", at+1)
		}
	}
}
