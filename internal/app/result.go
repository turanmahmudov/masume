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

// QueryStateKind says how far one run got.
type QueryStateKind string

// The four states a run can be in.
const (
	QueryIdle      QueryStateKind = "idle"
	QueryRunning   QueryStateKind = "running"
	QuerySucceeded QueryStateKind = "succeeded"
	QueryFailed    QueryStateKind = "failed"
)

// QueryState is what the pane knows about the run of the active statement.
type QueryState struct {
	Kind    QueryStateKind
	Result  db.QueryResult
	Message string
}

// PlanStateKind says how far the plan of a statement got.
type PlanStateKind string

// The four states a plan can be in.
const (
	PlanNone    PlanStateKind = "none"
	PlanLoading PlanStateKind = "loading"
	PlanReady   PlanStateKind = "ready"
	PlanFailed  PlanStateKind = "failed"
)

// PlanState is the plan of one statement, and how far the read of it got.
type PlanState struct {
	Kind    PlanStateKind
	Plan    query.QueryPlan
	Message string
}

// StatementResult is one statement of a batch, and everything read for it.
type StatementResult struct {
	ID int
	// The label the strip of a batch draws, which is the first words of the statement.
	Label string
	// The statement the run was built from, before any rewrite.
	Source string
	// The read the engine composed, which paging and counting go through.
	Read  db.ComposedRead
	State QueryState
	// Revision counts the times the rows of this statement changed, so a reader that
	// keeps what it built from them knows when to build it again.
	Revision int
	Plan     PlanState
	// How many rows the whole result holds, once the user asked for the total.
	TotalRows    int64
	HasTotalRows bool
	// True while the next page is being read.
	FetchingMore bool
	// True while the whole result is being counted, so the size says a count is on its way.
	Counting bool
	// The rows a page holds, which is the step size through the relation.
	PageSize int
	// When the run of this statement began, so the wheel beside it can count the seconds.
	StartedAt time.Time
	// When the answer landed, which the statistics report beside the start.
	FinishedAt time.Time
}

// BuildStatementLabel writes the label of one statement of a batch: the relation it reads,
// and not the text of the statement.
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

// resolveStatementLabel keeps two results of one relation apart, as `orders` and `orders (2)`.
func resolveStatementLabel(written string, taken map[string]bool) string {
	base := BuildStatementLabel(written)
	label := base
	for count := 2; taken[label]; count++ {
		label = base + " (" + strconv.Itoa(count) + ")"
	}
	taken[label] = true
	return label
}

// ResultStore holds one result per statement, and which of them the pane draws.
type ResultStore struct {
	results     []*StatementResult
	activeIndex int
	nextID      int
}

// NewResultStore starts a store with nothing read yet.
func NewResultStore() *ResultStore {
	return &ResultStore{}
}

// Results returns every statement of the last run.
func (store *ResultStore) Results() []*StatementResult {
	return store.results
}

// ActiveIndex returns which statement the pane draws.
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

// ResultAt returns the statement at that position, and nothing where the run holds none.
func (store *ResultStore) ResultAt(index int) *StatementResult {
	if index < 0 || index >= len(store.results) {
		return nil
	}
	return store.results[index]
}

// State returns how far the active statement got.
func (store *ResultStore) State() QueryState {
	active := store.Active()
	if active == nil {
		return QueryState{Kind: QueryIdle}
	}
	return active.State
}

// Start clears the store and opens one entry per statement of the run.
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

// IsRunning is true while any statement of the run still waits for the server.
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

// SelectNextResult steps to the statement before or after the one drawn.
func (store *ResultStore) SelectNextResult(step int) {
	store.activeIndex = core.WrapIndex(store.activeIndex+step, len(store.results))
}

// Succeed keeps what one statement answered.
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

// Fail keeps why one statement did not run.
func (store *ResultStore) Fail(index int, message string) {
	if index < 0 || index >= len(store.results) {
		return
	}
	store.results[index].State = QueryState{Kind: QueryFailed, Message: message}
	store.results[index].FinishedAt = time.Now()
}

// SkipRest marks every statement from this one on as never run, which a batch stopped by a
// failure leaves behind. Without it they wait for a server that is never asked.
func (store *ResultStore) SkipRest(from int, message string) {
	for index := from; index < len(store.results); index++ {
		if store.results[index].State.Kind != QueryRunning {
			continue
		}
		store.results[index].State = QueryState{Kind: QueryFailed, Message: message}
		store.results[index].FinishedAt = time.Now()
	}
}

// IsCounting is true while any statement of the tab is being counted.
func (store *ResultStore) IsCounting() bool {
	for _, held := range store.results {
		if held.Counting {
			return true
		}
	}
	return false
}

// AppendRows adds the next page to the result already drawn.
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
	result.Rows = append(append([][]any{}, result.Rows...), page.Rows...)
	result.Truncated = page.Truncated
	if len(result.Columns) == 0 {
		result.Columns = page.Columns
	}
	held.State.Result = result
	held.Revision++
}

// CanFetchMore is true where the read stopped at a page and the server holds more.
func (store *ResultStore) CanFetchMore() bool {
	active := store.Active()
	if active == nil || active.State.Kind != QuerySucceeded {
		return false
	}
	return active.State.Result.Truncated && active.Read.Pageable
}

// CanCountRows is true where the server can count the whole result without running the read
// twice.
func (store *ResultStore) CanCountRows() bool {
	active := store.Active()
	if active == nil || active.State.Kind != QuerySucceeded {
		return false
	}
	return active.Read.Pageable
}
