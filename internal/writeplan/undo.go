package writeplan

import (
	"context"
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/build"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// UndoPlan is the undo availability and query for original rows. Capture uses the write transaction and dialect row locking.
type UndoPlan struct {
	// Kept is true where the rows are read and the write can be undone.
	Kept bool
	// Reason is the cause of unavailable undo.
	Reason string
	// Rows is the measured count for undo capture.
	Rows  int64
	Table db.TableRef
	Kind  statement.WriteKind
	// Read is the capture query. Limit is the maximum rows to capture.
	Read  string
	Limit int
	// Keys is the primary key. Columns is the query column list.
	Keys    []string
	Columns []string
	// dialect writes the undo statements for the server of the write.
	dialect *query.Dialect
}

// Undo is a set of statements built from original rows captured in the write transaction.
type Undo struct {
	Table   db.TableRef
	Changes []db.Change
	// The same statements with their values written in.
	Display []string
	Rows    int
	// The reason undo is unavailable.
	Reason string
}

// IsHeld is true where the write can be undone.
func (undo Undo) IsHeld() bool { return len(undo.Changes) > 0 }

// planUndo checks undo availability and builds the capture query. Capture occurs before write execution.
func (measure measurer) planUndo(ctx context.Context, plan Plan, undoRows int) UndoPlan {
	refuse := func(reason string) UndoPlan {
		return UndoPlan{Reason: reason, Table: measure.table, Kind: measure.target.Kind}
	}
	if measure.target.Kind == statement.WriteInsert {
		return refuse("inserted rows are unknown before execution")
	}
	if !plan.HasRows {
		return refuse("the rows were not counted")
	}
	if plan.Rows == 0 {
		return refuse("the write matches no rows")
	}
	if undoRows > 0 && plan.Rows > int64(undoRows) {
		return refuse(fmt.Sprintf("%s rows, over the undo_rows limit of %d",
			present.FormatCount(plan.Rows), undoRows))
	}

	detail, err := measure.session.DescribeTable(ctx, measure.table)
	if err != nil {
		return refuse(db.DescribeError(err))
	}
	keys := readKeyColumns(detail)
	if len(keys) == 0 {
		return refuse("the table has no primary key to identify individual rows")
	}
	columns, reason := measure.buildUndoColumns(detail, keys)
	if reason != "" {
		return refuse(reason)
	}

	return UndoPlan{
		Kept: true, Rows: plan.Rows, Table: measure.table, Kind: measure.target.Kind,
		Read: measure.buildUndoRead(columns), Limit: undoRows,
		Keys: keys, Columns: columns, dialect: measure.dialect(),
	}
}

// buildUndoRead builds the capture query with the dialect row lock clause.
func (measure measurer) buildUndoRead(columns []string) string {
	dialect := measure.dialect()
	quoted := make([]string, 0, len(columns))
	for _, name := range columns {
		quoted = append(quoted, dialect.QuoteIdentifier(name))
	}
	return "select " + strings.Join(quoted, ", ") + " from " + measure.quotedTable() +
		measure.buildPredicate() + dialect.RowLockClause
}

func readKeyColumns(detail db.TableDetail) []string {
	keys := []string{}
	for _, column := range detail.Columns {
		if column.IsPrimaryKey {
			keys = append(keys, column.Name)
		}
	}
	return keys
}

func findColumn(detail db.TableDetail, name string) (db.ColumnDetail, bool) {
	for _, column := range detail.Columns {
		if strings.EqualFold(column.Name, name) {
			return column, true
		}
	}
	return db.ColumnDetail{}, false
}

// buildUndoColumns returns keys and assigned columns for updates, or nongenerated columns for deletes.
func (measure measurer) buildUndoColumns(
	detail db.TableDetail, keys []string,
) ([]string, string) {
	if measure.target.Kind != statement.WriteUpdate {
		read := []string{}
		for _, column := range detail.Columns {
			if !column.IsGenerated {
				read = append(read, column.Name)
			}
		}
		return read, ""
	}

	read := append([]string{}, keys...)
	for _, name := range measure.target.Columns {
		column, held := findColumn(detail, name)
		if !held {
			return nil, fmt.Sprintf("the write assigns unknown column %q", name)
		}
		if column.IsPrimaryKey {
			return nil, "the write changes the primary key; undo cannot identify the original rows"
		}
		read = append(read, column.Name)
	}
	return read, ""
}

// undoRowCeiling is the capture limit for an unlimited profile. QueryRunner requires a finite limit to detect truncation.
const undoRowCeiling = 1 << 20

func resolveUndoLimit(limit int) int {
	if limit <= 0 {
		return undoRowCeiling
	}
	return limit
}

// ReadUndo captures original rows and builds undo statements inside the write transaction.
func ReadUndo(ctx context.Context, runner db.QueryRunner, plan UndoPlan) (Undo, error) {
	if !plan.Kept {
		return Undo{Table: plan.Table, Reason: plan.Reason}, nil
	}

	answered, err := runner.RunQuery(ctx, plan.Read, resolveUndoLimit(plan.Limit), nil)
	if err != nil {
		return Undo{}, err
	}
	if answered.Truncated {
		return Undo{}, db.NewDatabaseError(
			"the write matches more rows than the undo_rows limit of %d", plan.Limit)
	}
	return buildUndoStatements(plan, answered.Rows, answered.Columns)
}

func buildUndoStatements(
	plan UndoPlan, rows [][]any, columns []db.ResultColumn,
) (Undo, error) {
	target := build.WriteTarget{
		Table: plan.Table.Qualified(), Columns: columns, KeyColumns: plan.Keys,
		Dialect: plan.dialect,
	}
	restored := findRestoredColumns(columns, plan.Keys)

	undo := Undo{Table: plan.Table, Rows: len(rows)}
	for _, row := range rows {
		bound, shown, err := buildUndoRow(target, row, restored, plan.Kind)
		if err != nil {
			return Undo{}, err
		}
		undo.Changes = append(undo.Changes, db.Change{
			Description: bound.Description, Display: bound.SQL,
			Params: bound.Params, Payload: bound,
		})
		undo.Display = append(undo.Display, shown)
	}
	return undo, nil
}

// buildUndoRow returns the undo of one row, bound and written out.
func buildUndoRow(
	target build.WriteTarget, row []any, restored []int, kind statement.WriteKind,
) (query.BoundStatement, string, error) {
	if kind == statement.WriteUpdate {
		bound, err := build.BuildUndoUpdate(target, row, restored)
		if err != nil {
			return query.BoundStatement{}, "", err
		}
		shown, err := build.BuildShownUndoUpdate(target, row, restored)
		return bound, shown, err
	}
	bound, err := build.BuildUndoInsert(target, row)
	if err != nil {
		return query.BoundStatement{}, "", err
	}
	shown, err := build.BuildShownUndoInsert(target, row)
	return bound, shown, err
}

func findRestoredColumns(columns []db.ResultColumn, keys []string) []int {
	isKey := map[string]bool{}
	for _, name := range keys {
		isKey[strings.ToLower(name)] = true
	}
	restored := []int{}
	for at, column := range columns {
		if !isKey[strings.ToLower(column.Name)] {
			restored = append(restored, at)
		}
	}
	return restored
}
