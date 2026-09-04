package app_test

import (
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
)

// describeRows returns the tree as one line per row, so a case states the shape it wants
// rather than a field at a time.
func describeRows(rows []app.BlockingNode) []string {
	said := make([]string, 0, len(rows))
	for _, row := range rows {
		mark := "holds"
		if row.Waiting {
			mark = "waits"
		}
		said = append(said, mark+" "+row.Query+" at "+string(rune('0'+row.Depth)))
	}
	return said
}

func buildWait(blocking, blocked int64, waiting time.Duration) db.LockWait {
	return db.LockWait{
		BlockingPID: blocking, BlockingQuery: "held-" + string(rune('a'+blocking)),
		BlockingFor: 4 * time.Minute,
		BlockedPID:  blocked, BlockedQuery: "wait-" + string(rune('a'+blocked)),
		Waiting: waiting, Mode: "AccessExclusiveLock", Relation: "orders",
	}
}

// The tree is what the person on call reads first: one session holds a lock and the others
// stand behind it. The server answers a flat list of pairs, so the shape has to be built.
func TestBuildBlockingTreeShapesWhatTheServerAnswered(t *testing.T) {
	for _, held := range []struct {
		name  string
		waits []db.LockWait
		want  []string
	}{
		{"no waits at all", nil, nil},
		{
			name:  "one holder and one waiter",
			waits: []db.LockWait{buildWait(1, 2, time.Minute)},
			want:  []string{"holds held-b at 0", "waits wait-c at 1"},
		},
		{
			name: "one holder and two waiters, the longest wait first",
			waits: []db.LockWait{
				buildWait(1, 2, 2*time.Minute),
				buildWait(1, 3, time.Minute),
			},
			want: []string{
				"holds held-b at 0",
				"waits wait-c at 1",
				"waits wait-d at 1",
			},
		},
		{
			// The middle session both waits and blocks. It must appear once, under the
			// session it waits for, or the reader cannot see where the chain starts.
			name: "a chain of three",
			waits: []db.LockWait{
				buildWait(1, 2, 2*time.Minute),
				buildWait(2, 3, time.Minute),
			},
			want: []string{
				"holds held-b at 0",
				"waits wait-c at 1",
				"waits wait-d at 2",
			},
		},
		{
			name: "two separate trees",
			waits: []db.LockWait{
				buildWait(5, 6, time.Minute),
				buildWait(1, 2, time.Minute),
			},
			want: []string{
				"holds held-b at 0", "waits wait-c at 1",
				"holds held-f at 0", "waits wait-g at 1",
			},
		},
		{
			// Two holders block the same waiter. The waiter is one session and is drawn
			// once, under the first holder that reaches it.
			name: "one waiter behind two holders",
			waits: []db.LockWait{
				buildWait(1, 3, time.Minute),
				buildWait(2, 3, time.Minute),
			},
			want: []string{
				"holds held-b at 0", "waits wait-d at 1",
				"holds held-c at 0",
			},
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			rows := describeRows(app.BuildBlockingTree(held.waits))
			if len(rows) != len(held.want) {
				t.Fatalf("the tree reads %v, wanted %v", rows, held.want)
			}
			for at, row := range rows {
				if row != held.want[at] {
					t.Errorf("row %d reads %q, wanted %q", at, row, held.want[at])
				}
			}
		})
	}
}

// Two sessions can wait for each other. The server reports it, so the client must draw it
// rather than walk it forever. This is the case that would hang the whole client.
func TestBuildBlockingTreeDrawsACycleOnce(t *testing.T) {
	for _, held := range []struct {
		name  string
		waits []db.LockWait
	}{
		{"two sessions waiting for each other", []db.LockWait{
			buildWait(1, 2, time.Minute), buildWait(2, 1, time.Minute),
		}},
		{"three sessions in a ring", []db.LockWait{
			buildWait(1, 2, time.Minute),
			buildWait(2, 3, time.Minute),
			buildWait(3, 1, time.Minute),
		}},
		{"a session waiting for itself", []db.LockWait{buildWait(1, 1, time.Minute)}},
	} {
		t.Run(held.name, func(t *testing.T) {
			rows := app.BuildBlockingTree(held.waits)
			if len(rows) == 0 {
				t.Fatal("a cycle drew no rows, so the reader cannot see it")
			}
			seen := map[int64]bool{}
			for _, row := range rows {
				if seen[row.PID] {
					t.Errorf("session %d is drawn twice", row.PID)
				}
				seen[row.PID] = true
			}
		})
	}
}

// The same waits must always draw the same tree, whatever order the server answered them
// in. A tree that reorders itself between two refreshes cannot be read.
func TestBuildBlockingTreeIsTheSameWhateverTheOrder(t *testing.T) {
	forwards := []db.LockWait{
		buildWait(1, 2, time.Minute),
		buildWait(1, 3, time.Minute),
		buildWait(5, 6, time.Minute),
	}
	backwards := []db.LockWait{forwards[2], forwards[1], forwards[0]}

	first := app.BuildBlockingTree(forwards)
	second := app.BuildBlockingTree(backwards)
	if len(first) != len(second) {
		t.Fatalf("one order gives %d rows and the other %d", len(first), len(second))
	}
	for at := range first {
		if first[at].PID != second[at].PID {
			t.Errorf("row %d is session %d one way and %d the other",
				at, first[at].PID, second[at].PID)
		}
	}
}

// The summary line counts the sessions that are waiting, not the pairs the server answered.
// A session blocked by two holders is one session waiting.
func TestCountBlockedSessionsCountsSessionsAndNotPairs(t *testing.T) {
	for _, held := range []struct {
		name  string
		waits []db.LockWait
		want  int
	}{
		{"none", nil, 0},
		{"one", []db.LockWait{buildWait(1, 2, time.Minute)}, 1},
		{"two behind one holder", []db.LockWait{
			buildWait(1, 2, time.Minute), buildWait(1, 3, time.Minute),
		}, 2},
		{"one behind two holders", []db.LockWait{
			buildWait(1, 3, time.Minute), buildWait(2, 3, time.Minute),
		}, 1},
	} {
		t.Run(held.name, func(t *testing.T) {
			if counted := app.CountBlockedSessions(held.waits); counted != held.want {
				t.Errorf("the summary counts %d waiting, wanted %d", counted, held.want)
			}
		})
	}
}
