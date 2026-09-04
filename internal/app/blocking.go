package app

import (
	"cmp"
	"slices"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
)

// The tree of which session waits for which. The server answers one row per waiter and the
// session that blocks it, and the dashboard draws that as a tree.

// BlockingNode is one row of the tree, already flattened in the order it is drawn.
type BlockingNode struct {
	PID   int64
	Query string
	// How long this session has been running, or waiting where it is a waiter.
	Elapsed time.Duration
	// The lock it waits for and the relation the lock is on, or the lock a root holds.
	Mode     string
	Relation string
	Depth    int
	// True for a session that is waiting, false for the root that holds what they wait for.
	Waiting bool
}

// BuildBlockingTree returns the rows of the tree the waits describe, in the order they are
// drawn. A session that blocks one waiter and waits for another appears once, under the
// session it waits for.
//
// A cycle has no session that waits for nothing, so its lowest PID is taken as its root.
func BuildBlockingTree(waits []db.LockWait) []BlockingNode {
	if len(waits) == 0 {
		return nil
	}

	waitingFor := map[int64][]db.LockWait{}
	blocked := map[int64]bool{}
	for _, wait := range waits {
		waitingFor[wait.BlockingPID] = append(waitingFor[wait.BlockingPID], wait)
		blocked[wait.BlockedPID] = true
	}
	// The session that has waited longest is drawn first, and two that have waited the
	// same go by PID, so the same state draws the same tree every refresh.
	for holder := range waitingFor {
		slices.SortStableFunc(waitingFor[holder], func(first, second db.LockWait) int {
			if longest := cmp.Compare(second.Waiting, first.Waiting); longest != 0 {
				return longest
			}
			return cmp.Compare(first.BlockedPID, second.BlockedPID)
		})
	}

	rows := []BlockingNode{}
	drawn := map[int64]bool{}
	for _, holder := range listHolders(waitingFor) {
		if blocked[holder] || drawn[holder] {
			continue
		}
		rows = appendBlockingRows(rows, holder, waitingFor, drawn, 0)
	}
	// The rest is a cycle, or a chain hanging off one, rooted at its lowest PID.
	for _, holder := range listHolders(waitingFor) {
		if drawn[holder] {
			continue
		}
		rows = appendBlockingRows(rows, holder, waitingFor, drawn, 0)
	}
	return rows
}

// listHolders returns every session that blocks another, lowest PID first.
func listHolders(waitingFor map[int64][]db.LockWait) []int64 {
	holders := make([]int64, 0, len(waitingFor))
	for holder := range waitingFor {
		holders = append(holders, holder)
	}
	slices.Sort(holders)
	return holders
}

// appendBlockingRows writes the session and everything waiting under it. A session already
// drawn is not walked again, which is what stops a cycle.
func appendBlockingRows(
	rows []BlockingNode, holder int64, waitingFor map[int64][]db.LockWait,
	drawn map[int64]bool, depth int,
) []BlockingNode {
	held := waitingFor[holder]
	if depth == 0 {
		drawn[holder] = true
		rows = append(rows, buildHolderRow(holder, held))
	}

	for _, wait := range held {
		if drawn[wait.BlockedPID] {
			continue
		}
		drawn[wait.BlockedPID] = true
		rows = append(rows, BlockingNode{
			PID: wait.BlockedPID, Query: wait.BlockedQuery, Elapsed: wait.Waiting,
			Mode: wait.Mode, Relation: wait.Relation,
			Depth: depth + 1, Waiting: true,
		})
		rows = appendBlockingRows(rows, wait.BlockedPID, waitingFor, drawn, depth+1)
	}
	return rows
}

// buildHolderRow returns the row of the session that holds what the others wait for.
func buildHolderRow(holder int64, waits []db.LockWait) BlockingNode {
	row := BlockingNode{PID: holder}
	if len(waits) > 0 {
		row.Query = waits[0].BlockingQuery
		row.Elapsed = waits[0].BlockingFor
		row.Mode = waits[0].Mode
		row.Relation = waits[0].Relation
	}
	return row
}

// CountBlockedSessions returns how many sessions are waiting for a lock, counting one
// blocked by several holders once.
func CountBlockedSessions(waits []db.LockWait) int {
	blocked := map[int64]bool{}
	for _, wait := range waits {
		blocked[wait.BlockedPID] = true
	}
	return len(blocked)
}
