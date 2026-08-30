package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
)

func TestRunBatchesStopEveryRunOfAConnection(t *testing.T) {
	runs := runBatches{}
	first := runs.start(batchKey{connectionID: 1, tabID: 1},
		&runBatch{reads: []db.ComposedRead{{Text: "select 1"}}})
	second := runs.start(batchKey{connectionID: 1, tabID: 2},
		&runBatch{reads: []db.ComposedRead{{Text: "select 2"}}})
	other := runs.start(batchKey{connectionID: 2, tabID: 1},
		&runBatch{reads: []db.ComposedRead{{Text: "select 3"}}})

	runs.stopConnection(1)
	if _, held := runs.find(batchKey{connectionID: 1, tabID: 1}, first); held {
		t.Error("the first tab of the closed connection still has a run")
	}
	if _, held := runs.find(batchKey{connectionID: 1, tabID: 2}, second); held {
		t.Error("the second tab of the closed connection still has a run")
	}
	if _, held := runs.find(batchKey{connectionID: 2, tabID: 1}, other); !held {
		t.Error("stopping one connection stopped the run of another")
	}
	if runs.count() != 1 {
		t.Errorf("the client holds %d runs, wanted the one of the connection that stayed",
			runs.count())
	}
}

func TestRunBatchesFindRejectsAReplacedRun(t *testing.T) {
	runs := runBatches{}
	key := batchKey{connectionID: 1, tabID: 1}
	first := runs.start(key, &runBatch{reads: []db.ComposedRead{{Text: "select 1"}}})
	second := runs.start(key, &runBatch{reads: []db.ComposedRead{{Text: "select 2"}}})

	if _, held := runs.find(key, first); held {
		t.Error("the run that was replaced still answers")
	}
	if _, held := runs.find(key, second); !held {
		t.Error("the run in hand does not answer")
	}
	if _, held := runs.find(batchKey{connectionID: 1, tabID: 9}, second); held {
		t.Error("a tab with no run answered")
	}

	runs.stop(key)
	if _, held := runs.find(key, second); held {
		t.Error("a stopped run still answers")
	}
	if runs.count() != 0 {
		t.Errorf("the client holds %d runs after a stop, wanted none", runs.count())
	}
}
