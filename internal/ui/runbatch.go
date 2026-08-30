package ui

import (
	"github.com/turanmahmudov/masume/internal/db"
)

type runBatch struct {
	runID       int
	reads       []db.ComposedRead
	rowLimit    int
	profileName string
}

// batchKey names the tab a run belongs to. A connection carries a run per tab, so a run
// started in one tab never takes the place of the run of another.
type batchKey struct {
	connectionID int
	tabID        int
}

type runBatches struct {
	// The statements still to run, one entry per tab of a connection, so an answer can ask
	// for the next one and a failure can stop the rest. A run of one tab never replaces the
	// run of another.
	held map[batchKey]*runBatch
	// The number the next run is stamped with, so an answer of a run that was replaced is
	// dropped rather than written into the run that took its place.
	nextRunID int
}

// start opens a run of that tab and returns the number it is stamped with. The number
// tells the answers of this run from the answers of the run it replaced.
func (runs *runBatches) start(key batchKey, batch *runBatch) int {
	runs.nextRunID++
	batch.runID = runs.nextRunID
	if runs.held == nil {
		runs.held = map[batchKey]*runBatch{}
	}
	runs.held[key] = batch
	return batch.runID
}

func (runs *runBatches) find(key batchKey, runID int) (*runBatch, bool) {
	batch, held := runs.held[key]
	if !held || batch.runID != runID {
		return nil, false
	}
	return batch, true
}

func (runs *runBatches) stop(key batchKey) {
	delete(runs.held, key)
}

func (runs *runBatches) stopConnection(connectionID int) {
	for key := range runs.held {
		if key.connectionID == connectionID {
			delete(runs.held, key)
		}
	}
}

func (runs *runBatches) count() int { return len(runs.held) }
