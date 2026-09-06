package writeplan

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// Plain text write plans for chat and agent confirmations.

// The labels of the plan, which the card and the text both write.
const (
	LabelRows     = "rows"
	LabelColumns  = "columns"
	LabelCascades = "cascades"
	LabelBlocked  = "blocked"
	LabelUndo     = "undo"
	LabelCommit   = "commit"
)

// DescribeRows returns matching and total row counts.
func DescribeRows(plan Plan) string {
	if !plan.HasRows {
		return "not counted · " + plan.RowsReason
	}
	counted := present.FormatCount(plan.Rows)
	if !plan.HasTotal {
		return counted + " in " + plan.Table.Name
	}
	written := counted + " of " + present.FormatCount(plan.Total) + " in " + plan.Table.Name
	if plan.NamesEveryRow() {
		return written + " · every row"
	}
	if share, held := plan.ReadShare(); held {
		return written + " · " + core.FormatShare(share)
	}
	return written
}

// DescribeColumns returns assigned columns for an update.
func DescribeColumns(plan Plan) (string, bool) {
	if plan.Kind != statement.WriteUpdate {
		return "", false
	}
	return strings.Join(plan.Columns, ", "), true
}

// DescribeCascade describes a trigger or foreign key effect.
func DescribeCascade(cascade Cascade) string {
	written := cascade.Reason
	if cascade.Table != "" {
		written = cascade.Table + " · " + cascade.Reason
	}
	if cascade.HasRows {
		return written + " · " + present.FormatCountOf(cascade.Rows, "row", "rows")
	}
	return written
}

// DescribeBlocker describes a table with references that may block the write.
func DescribeBlocker(blocker Cascade) string {
	written := blocker.Table + " · " + blocker.Reason
	if !blocker.HasRows {
		return written + " · referencing rows not counted"
	}
	return written + " · " + present.FormatCountOf(blocker.Rows, "row", "rows") +
		" reference matching rows"
}

// DescribeUndo describes the original rows available for undo.
func DescribeUndo(undo UndoPlan) string {
	if !undo.Kept {
		return "none · " + undo.Reason
	}
	return present.FormatCountOf(undo.Rows, "row", "rows") + " to capture before the write"
}

// DescribeCommit returns how the write is committed.
func DescribeCommit(plan Plan) string {
	if plan.InTransaction {
		return "uses the open transaction"
	}
	if plan.Undo.Kept {
		return "the write and undo capture use one transaction"
	}
	return "autocommit"
}

// DescribeLines returns the whole plan as lines of text.
func DescribeLines(plan Plan) []string {
	lines := []string{writeLine(LabelRows, DescribeRows(plan))}
	if columns, named := DescribeColumns(plan); named {
		lines = append(lines, writeLine(LabelColumns, columns))
	}
	for at, cascade := range plan.Cascades {
		label := LabelCascades
		if at > 0 {
			label = ""
		}
		lines = append(lines, writeLine(label, DescribeCascade(cascade)))
	}
	for at, blocker := range plan.Blockers {
		label := LabelBlocked
		if at > 0 {
			label = ""
		}
		lines = append(lines, writeLine(label, DescribeBlocker(blocker)))
	}
	return append(lines,
		writeLine(LabelUndo, DescribeUndo(plan.Undo)),
		writeLine(LabelCommit, DescribeCommit(plan)))
}

// labelWidth is the column the values of the plan start in.
const labelWidth = 10

func writeLine(label, value string) string {
	return label + strings.Repeat(" ", max(labelWidth-len(label), 1)) + value
}
