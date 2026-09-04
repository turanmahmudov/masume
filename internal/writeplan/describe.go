package writeplan

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The plan as plain text, for a caller that draws no card: the chat panel, and the
// question an agent client asks its user.

// The labels of the plan, which the card and the text both write.
const (
	LabelRows     = "rows"
	LabelColumns  = "columns"
	LabelCascades = "cascades"
	LabelBlocked  = "blocked"
	LabelUndo     = "undo"
	LabelCommit   = "commit"
)

// DescribeRows returns the rows the write matches, and the rows the relation holds.
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

// DescribeColumns returns the columns an update assigns, and nothing for a write that
// names no column of its own.
func DescribeColumns(plan Plan) (string, bool) {
	if plan.Kind != statement.WriteUpdate {
		return "", false
	}
	return strings.Join(plan.Columns, ", "), true
}

// DescribeCascade returns one relation the write reaches through the server.
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

// DescribeBlocker returns one relation that blocks the write.
func DescribeBlocker(blocker Cascade) string {
	written := blocker.Table + " · " + blocker.Reason
	if !blocker.HasRows {
		return written + " · its rows were not counted"
	}
	return written + " · " + present.FormatCountOf(blocker.Rows, "row", "rows") +
		" reference these"
}

// DescribeUndo returns what the undo of the write will hold.
func DescribeUndo(undo UndoPlan) string {
	if !undo.Kept {
		return "none · " + undo.Reason
	}
	return present.FormatCountOf(undo.Rows, "row", "rows") + " read with the write"
}

// DescribeCommit returns how the write is committed.
func DescribeCommit(plan Plan) string {
	if plan.InTransaction {
		return "joins the transaction you hold open"
	}
	if plan.Undo.Kept {
		return "the write and its undo run in one transaction"
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
