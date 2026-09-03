package app

import (
	"strconv"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// QueryStateKind is the state of one run.
type QueryStateKind string

// The four states of a run.
const (
	QueryIdle      QueryStateKind = "idle"
	QueryRunning   QueryStateKind = "running"
	QuerySucceeded QueryStateKind = "succeeded"
	QueryFailed    QueryStateKind = "failed"
)

// QueryState is the state of the run of the active statement.
type QueryState struct {
	Kind    QueryStateKind
	Result  db.QueryResult
	Message string
}

// PlanStateKind is the state of the plan of a statement.
type PlanStateKind string

// The four states of a plan.
const (
	PlanNone    PlanStateKind = "none"
	PlanLoading PlanStateKind = "loading"
	PlanReady   PlanStateKind = "ready"
	PlanFailed  PlanStateKind = "failed"
)

// PlanState is the plan of one statement and the state of its read.
type PlanState struct {
	Kind    PlanStateKind
	Plan    query.QueryPlan
	Message string
}

// StatementResult is one statement of a batch and everything read for it.
type StatementResult struct {
	ID int
	// The label the batch strip draws, which is the first words of the statement.
	Label string
	// The statement the run was built from, before a rewrite.
	Source string
	// The read the engine composed. Paging and counting use it.
	Read  db.ComposedRead
	State QueryState
	// Revision counts how often the rows of this statement were replaced, so a caller
	// that caches a result of the rows knows when to build it again. A page added at the
	// end does not change it: the rows before the page are unchanged, and their number
	// says how many rows the caller already read.
	Revision int
	Plan     PlanState
	// The number of rows of the whole result, after the user requested the total.
	TotalRows    int64
	HasTotalRows bool
	// True while the next page is read.
	FetchingMore bool
	// True while the whole result is counted, so the size shows that a count runs.
	Counting bool
	// The number of rows of a page, which is the step size through the table.
	PageSize int
	// The start time of the run, so the wait indicator can show the elapsed time.
	StartedAt time.Time
	// The time the answer arrived, which the statistics show next to the start time.
	FinishedAt time.Time
}

// BuildStatementLabel returns the label of one statement of a batch: the table it reads, and
// not the text of the statement.
func BuildStatementLabel(written string) string {
	references := statement.FindTableReferences(written, syntax.FlavourStandard)
	if len(references) > 0 {
		return references[0].Name
	}
	if word := syntax.ReadCommandWord(written, syntax.FlavourStandard); word != "" {
		return word
	}
	return "statement"
}

// resolveStatementLabel separates two results of one table, as `orders` and `orders (2)`.
func resolveStatementLabel(written string, taken map[string]bool) string {
	base := BuildStatementLabel(written)
	label := base
	for count := 2; taken[label]; count++ {
		label = base + " (" + strconv.Itoa(count) + ")"
	}
	taken[label] = true
	return label
}

// ResultStore holds one result per statement and the index of the result the pane draws.
type ResultStore struct {
	results     []*StatementResult
	activeIndex int
	nextID      int
}

// NewResultStore returns an empty store.
func NewResultStore() *ResultStore {
	return &ResultStore{}
}

// Results returns every statement of the last run.
func (store *ResultStore) Results() []*StatementResult {
	return store.results
}

// ActiveIndex returns the index of the statement the pane draws.
func (store *ResultStore) ActiveIndex() int {
	return store.activeIndex
}

// Active returns the statement the pane draws, and nothing before the first run.
func (store *ResultStore) Active() *StatementResult {
	if store.activeIndex < 0 || store.activeIndex >= len(store.results) {
		return nil
	}
	return store.results[store.activeIndex]
}

// ResultAt returns the statement at that index, and nothing if the run has none.
func (store *ResultStore) ResultAt(index int) *StatementResult {
	if index < 0 || index >= len(store.results) {
		return nil
	}
	return store.results[index]
}

// State returns the state of the active statement.
func (store *ResultStore) State() QueryState {
	active := store.Active()
	if active == nil {
		return QueryState{Kind: QueryIdle}
	}
	return active.State
}

// Start clears the store and creates one entry per statement of the run.
func (store *ResultStore) Start(statements []string, pageSize int) {
	store.results = make([]*StatementResult, 0, len(statements))
	taken := map[string]bool{}
	startedAt := time.Now()
	for _, written := range statements {
		store.nextID++
		store.results = append(store.results, &StatementResult{
			ID: store.nextID, Label: resolveStatementLabel(written, taken), Source: written,
			State: QueryState{Kind: QueryRunning}, PageSize: pageSize, StartedAt: startedAt,
		})
	}
	store.activeIndex = 0
}

// IsRunning is true while a statement of the run waits for the server.
func (store *ResultStore) IsRunning() bool {
	for _, held := range store.results {
		if held.State.Kind == QueryRunning || held.FetchingMore {
			return true
		}
	}
	return false
}

// SelectResult moves the pane to another statement of a batch.
func (store *ResultStore) SelectResult(index int) {
	store.activeIndex = core.ClampIndex(index, len(store.results))
}

// SelectNextResult moves to the statement before or after the one that is drawn.
func (store *ResultStore) SelectNextResult(step int) {
	store.activeIndex = core.WrapIndex(store.activeIndex+step, len(store.results))
}

// Succeed stores the answer of one statement.
func (store *ResultStore) Succeed(index int, read db.ComposedRead, result db.QueryResult) {
	if index < 0 || index >= len(store.results) {
		return
	}
	held := store.results[index]
	held.Read = read
	held.State = QueryState{Kind: QuerySucceeded, Result: result}
	held.TotalRows, held.HasTotalRows = 0, false
	held.Plan = PlanState{Kind: PlanNone}
	held.FinishedAt = time.Now()
	held.Revision++
}

// Fail stores the reason one statement did not run.
func (store *ResultStore) Fail(index int, message string) {
	if index < 0 || index >= len(store.results) {
		return
	}
	store.results[index].State = QueryState{Kind: QueryFailed, Message: message}
	store.results[index].FinishedAt = time.Now()
}

// SkipRest marks every statement from this one on as not run, which is the state after a
// batch stopped at an error. Without it they wait for a server that is never asked.
func (store *ResultStore) SkipRest(from int, message string) {
	for index := from; index < len(store.results); index++ {
		if store.results[index].State.Kind != QueryRunning {
			continue
		}
		store.results[index].State = QueryState{Kind: QueryFailed, Message: message}
		store.results[index].FinishedAt = time.Now()
	}
}

// IsCounting is true while a statement of the tab is counted.
func (store *ResultStore) IsCounting() bool {
	for _, held := range store.results {
		if held.Counting {
			return true
		}
	}
	return false
}

// AppendRows adds the next page to the result on screen.
func (store *ResultStore) AppendRows(index int, page db.QueryResult) {
	if index < 0 || index >= len(store.results) {
		return
	}
	held := store.results[index]
	held.FetchingMore = false
	if held.State.Kind != QuerySucceeded {
		return
	}
	result := held.State.Result
	// The rows already read stay in place, so the page is added at the end and the
	// revision does not change: a caller that formatted them keeps its result and
	// formats the new rows only. A result of many pages is then formatted one time and
	// not one time per page.
	result.Rows = append(result.Rows, page.Rows...)
	result.Truncated = page.Truncated
	if len(result.Columns) == 0 {
		result.Columns = page.Columns
		held.Revision++
	}
	held.State.Result = result
}

// CanFetchMore is true if the read stopped at a page and the server has more rows.
func (store *ResultStore) CanFetchMore() bool {
	active := store.Active()
	if active == nil || active.State.Kind != QuerySucceeded {
		return false
	}
	return active.State.Result.Truncated && active.Read.Pageable
}

// CanCountRows is true if the server can count the whole result without a second run of the
// read.
func (store *ResultStore) CanCountRows() bool {
	active := store.Active()
	if active == nil || active.State.Kind != QuerySucceeded {
		return false
	}
	return active.Read.Pageable
}
