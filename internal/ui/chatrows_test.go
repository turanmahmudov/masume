package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/hist"
)

// buildChatModel answers a model with a conversation of a few turns open in the chat panel.
func buildChatModel(t *testing.T) (*Model, *app.Chat) {
	t.Helper()
	model := buildOfflineModelFor(t, 160, 48)
	connection := model.Active()
	connection.Chat.Messages = []app.ChatMessage{
		{Role: hist.ChatRoleUser, Content: "which table holds the orders?"},
		{Role: hist.ChatRoleAssistant, Content: "public.orders holds one row per order."},
		{Role: hist.ChatRoleUser, Content: "and the customers?"},
		{Role: hist.ChatRoleAssistant, Content: "public.customers, one row per customer."},
	}
	connection.Overlay = app.Overlay{Kind: app.OverlayAiChat, Draft: app.NewEditorBuffer("", 0)}
	return model, connection.Chat
}

// readChatRows answers the rows the panel draws now, through the same path the frame takes.
func readChatRows(model *Model, connection *app.Connection) []string {
	rows, _ := model.resolveChatRows(connection, model.resolveChatContent())
	return rows
}

// markedChatRow is laid over the rows that were kept. A draw of the conversation writes the
// turns, so the mark is gone wherever the conversation was drawn again.
const markedChatRow = "marked"

// markKeptChatRows lays the mark over the rows the model holds.
func markKeptChatRows(model *Model) {
	model.chatRows.rows = []string{markedChatRow}
}

// holdsMarkedChatRows is true while the rows the model holds are still the marked ones.
func holdsMarkedChatRows(model *Model) bool {
	return model.chatRows.drawn && len(model.chatRows.rows) == 1 &&
		model.chatRows.rows[0] == markedChatRow
}

// The scroll bounds, a jump between turns and the draw all read the rows, so one press drew
// the whole conversation three times. It is drawn once and kept.
func TestChatRowsAreKeptWhileNothingChanges(t *testing.T) {
	model, _ := buildChatModel(t)
	first := readChatRows(model, model.Active())

	markKeptChatRows(model)
	for range 20 {
		readChatRows(model, model.Active())
	}
	if !holdsMarkedChatRows(model) {
		t.Error("the conversation was drawn again although nothing changed")
	}

	model.chatRows.rows = first
	if len(readChatRows(model, model.Active())) != len(first) {
		t.Error("the conversation that was kept holds a different number of rows")
	}
}

// One test per input the rows are drawn from. A cache that misses one of these draws a
// conversation that is not the one on screen.
func TestChatRowsAreDrawnAgainForEveryChange(t *testing.T) {
	cases := []struct {
		name string
		// streaming asks for a reply to be written before the rows are first read,
		// because the calls and the wheel belong to one.
		streaming bool
		change    func(model *Model, chat *app.Chat)
	}{
		{name: "the panel is a different width", change: func(model *Model, _ *app.Chat) {
			model.width = 120
		}},
		{name: "the theme was changed", change: func(model *Model, _ *app.Chat) {
			model.styles.keepTheme(model.styles.Theme)
		}},
		{name: "a turn was added", change: func(_ *Model, chat *app.Chat) {
			chat.Messages = append(chat.Messages,
				app.ChatMessage{Role: hist.ChatRoleUser, Content: "and the roles?"})
		}},
		{name: "the text of a turn changed", change: func(_ *Model, chat *app.Chat) {
			chat.Messages[1].Content = "public.orders holds one row for each order placed."
		}},
		{
			name: "the text of a turn changed and kept its length",
			change: func(_ *Model, chat *app.Chat) {
				held := chat.Messages[1].Content
				chat.Messages[1].Content = strings.Repeat("x", len(held))
			},
		},
		{name: "the role of a turn changed", change: func(_ *Model, chat *app.Chat) {
			chat.Messages[1].Role = hist.ChatRoleUser
		}},
		{name: "a turn was marked", change: func(_ *Model, chat *app.Chat) {
			chat.HasTurn = true
		}},
		{name: "the mark moved to another turn", change: func(_ *Model, chat *app.Chat) {
			chat.HasTurn, chat.TurnAt = true, 2
		}},
		{name: "a reply began to stream", change: func(_ *Model, chat *app.Chat) {
			chat.Status = app.ChatStreaming
		}},
		{
			name: "a call of the reply finished", streaming: true,
			change: func(_ *Model, chat *app.Chat) {
				chat.Steps = []string{"read the catalog"}
			},
		},
		{
			name: "the reply began another call", streaming: true,
			change: func(_ *Model, chat *app.Chat) {
				chat.Activity = "reading public.orders"
			},
		},
		{
			name: "the reply began at another moment", streaming: true,
			change: func(_ *Model, chat *app.Chat) {
				chat.StartedAt = time.Unix(1_700_000_000, 0)
			},
		},
		{
			name: "the wheel of the reply turned", streaming: true,
			change: func(model *Model, _ *app.Chat) { model.spinnerAt++ },
		},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			model, chat := buildChatModel(t)
			if held.streaming {
				chat.Status = app.ChatStreaming
			}
			readChatRows(model, model.Active())

			markKeptChatRows(model)
			held.change(model, chat)
			readChatRows(model, model.Active())
			if holdsMarkedChatRows(model) {
				t.Error("the conversation was kept although " + held.name)
			}
		})
	}
}

// A cache that draws again and answers what it held before passes a test that only counts the
// draws, so the rows themselves are read.
func TestChatRowsDrawTheTextOfATurnThatChanged(t *testing.T) {
	model, chat := buildChatModel(t)
	readChatRows(model, model.Active())

	chat.Messages[1].Content = "the orders live in the shop schema"
	if !strings.Contains(strings.Join(readChatRows(model, model.Active()), "\n"), "shop schema") {
		t.Error("the rows do not hold the text the turn was changed to")
	}
	if strings.Contains(strings.Join(readChatRows(model, model.Active()), "\n"), "one row per order") {
		t.Error("the rows still hold the text the turn was changed from")
	}
}

// The wheel of a reply turns ten times a second and nothing else about it changes, so the rows
// of the turns before it are drawn again with it. A conversation with no reply being written
// keeps them whatever the wheel does.
func TestChatRowsAreKeptWhileNoReplyIsWritten(t *testing.T) {
	model, chat := buildChatModel(t)
	readChatRows(model, model.Active())

	markKeptChatRows(model)
	model.spinnerAt += 5
	chat.Steps = []string{"a call of a reply that ended"}
	chat.Activity = "something that ran before"
	readChatRows(model, model.Active())
	if !holdsMarkedChatRows(model) {
		t.Error("the conversation was drawn again although no reply is being written")
	}
}

// A jump between turns reads the row a turn begins on. It has to be the row the draw puts it
// on, or a jump lands somewhere else than the turn it named.
func TestFindChatTurnRowNamesTheRowTheTurnIsDrawnOn(t *testing.T) {
	model, chat := buildChatModel(t)
	connection := model.Active()
	rows, starts := model.resolveChatRows(connection, model.resolveChatContent())

	if len(starts) != len(chat.Messages) {
		t.Fatalf("the conversation answered %d turn rows for %d turns",
			len(starts), len(chat.Messages))
	}
	for turn := range chat.Messages {
		held := model.findChatTurnRow(connection, turn)
		if held != starts[turn] {
			t.Fatalf("turn %d was found on row %d, but it is drawn on row %d",
				turn, held, starts[turn])
		}
		if held >= len(rows) {
			t.Fatalf("turn %d was found on row %d, past the %d rows drawn",
				turn, held, len(rows))
		}
	}
	if model.findChatTurnRow(connection, 0) != 0 {
		t.Error("the first turn is not on the first row")
	}
	if model.findChatTurnRow(connection, len(chat.Messages)+4) !=
		starts[len(starts)-1] {
		t.Error("a turn past the last one is not held to the last turn")
	}
}

// The chat sends on Enter, because a question is asked far more often than it is written over
// several lines. A modifier writes the line.
func TestTheChatSendsOnEnterAndWritesALineOnAModifier(t *testing.T) {
	model, chat := buildChatModel(t)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayAiChat, Draft: app.NewEditorBuffer("how many orders", 15),
	}

	for _, held := range []struct {
		name string
		mod  uv.KeyMod
	}{{"shift", uv.ModShift}, {"alt", uv.ModAlt}} {
		t.Run(held.name+" writes a line", func(t *testing.T) {
			connection.Overlay.Draft = app.NewEditorBuffer("how many orders", 15)
			model.readOverlayKey(connection, tea.Key{Code: tea.KeyEnter, Mod: held.mod})
			if written := connection.Overlay.Draft.Text; written != "how many orders\n" {
				t.Errorf("the field holds %q", written)
			}
			if chat.IsStreaming() {
				t.Error("the question was sent")
			}
		})
	}

	connection.Overlay.Draft = app.NewEditorBuffer("how many orders", 15)
	model.readOverlayKey(connection, tea.Key{Code: tea.KeyEnter})
	if written := connection.Overlay.Draft.Text; written == "how many orders\n" {
		t.Errorf("Enter wrote a line instead of sending, and the field holds %q", written)
	}
}
