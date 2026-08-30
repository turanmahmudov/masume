package ui

import (
	"iter"

	"github.com/turanmahmudov/masume/internal/app"
)

type openConnection struct {
	id   int
	held *app.Connection
}

type openConnections struct {
	held    []openConnection
	current int
	// The id given to each connection, so an answer that arrives late is dropped.
	nextID int
}

func (list *openConnections) count() int { return len(list.held) }

func (list *openConnections) at(index int) *app.Connection {
	if index < 0 || index >= len(list.held) {
		return nil
	}
	return list.held[index].held
}

func (list *openConnections) idAt(index int) int {
	if index < 0 || index >= len(list.held) {
		return 0
	}
	return list.held[index].id
}

func (list *openConnections) all() iter.Seq2[int, *app.Connection] {
	return func(yield func(int, *app.Connection) bool) {
		for at, open := range list.held {
			if !yield(at, open.held) {
				return
			}
		}
	}
}

func (list *openConnections) activeIndex() int { return list.current }

func (list *openConnections) active() *app.Connection { return list.at(list.current) }

func (list *openConnections) activeID() int { return list.idAt(list.current) }

func (list *openConnections) focus(index int) { list.current = index }

func (list *openConnections) step(by int) {
	list.current = wrap(list.current+by, len(list.held))
}

func (list *openConnections) open(connection *app.Connection) int {
	list.nextID++
	list.held = append(list.held, openConnection{id: list.nextID, held: connection})
	list.current = len(list.held) - 1
	return list.nextID
}

func (list *openConnections) closeActive() (*app.Connection, int, bool) {
	at := list.current
	if at < 0 || at >= len(list.held) {
		return nil, 0, false
	}
	closed := list.held[at]
	list.held = append(list.held[:at], list.held[at+1:]...)
	list.current = clamp(at, len(list.held))
	return closed.held, closed.id, true
}

func (list *openConnections) find(id int) (*app.Connection, int, bool) {
	for at, open := range list.held {
		if open.id == id {
			return open.held, at, true
		}
	}
	return nil, 0, false
}

func (list *openConnections) idOf(connection *app.Connection) int {
	for _, open := range list.held {
		if open.held == connection {
			return open.id
		}
	}
	return 0
}

// holdsTab is true while that tab of that connection is open. It walks the open tabs rather
// than building a set of them, because a frame holds tens of tabs at the most and a set would
// be built and thrown away on every frame.
func (list *openConnections) holdsTab(key tabKey) bool {
	connection, _, open := list.find(key.connection)
	if !open {
		return false
	}
	for _, tab := range connection.Tabs {
		if tab.ID == key.tab {
			return true
		}
	}
	return false
}
