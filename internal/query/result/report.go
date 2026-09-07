package result

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/query"
)

// A report is a Markdown document of several statements and their rows: the prose between
// them, the statement of each one, and its rows as a table. One layout serves the screen
// and the run without one.

// ReportBlock is one part of a report: prose, or a statement and what it answered.
type ReportBlock struct {
	// Heading is the name of the block, drawn as a heading. Prose carries none.
	Heading string
	// Prose is written as it stands, with no fence around it.
	Prose string
	// SQL is the statement, drawn in a fence.
	SQL string
	// Fence is the language of a block that is neither prose nor a statement.
	Fence   string
	Columns []query.ResultColumn
	Rows    [][]any
	// Note is a line under the block, such as what a statement changed, or why it holds
	// no rows.
	Note string
}

// BuildReport returns the Markdown document of a title and its blocks.
func BuildReport(title string, blocks []ReportBlock) string {
	parts := []string{}
	if trimmed := strings.TrimSpace(title); trimmed != "" {
		parts = append(parts, "# "+trimmed)
	}
	for _, block := range blocks {
		if written := buildReportBlock(block); written != "" {
			parts = append(parts, written)
		}
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// buildReportBlock returns one block of a report.
func buildReportBlock(block ReportBlock) string {
	lines := []string{}
	if trimmed := strings.TrimSpace(block.Heading); trimmed != "" {
		lines = append(lines, "## "+trimmed)
	}
	if trimmed := strings.TrimSpace(block.Prose); trimmed != "" {
		lines = append(lines, trimmed)
	}
	if trimmed := strings.TrimSpace(block.SQL); trimmed != "" {
		fence := block.Fence
		if fence == "" {
			fence = "sql"
		}
		lines = append(lines, "```"+fence+"\n"+trimmed+"\n```")
	}
	if len(block.Columns) > 0 {
		lines = append(lines,
			strings.TrimRight(BuildMarkdown(block.Columns, block.Rows), "\n"))
	}
	if trimmed := strings.TrimSpace(block.Note); trimmed != "" {
		lines = append(lines, "_"+trimmed+"_")
	}
	return strings.Join(lines, "\n\n")
}
