// Package headless runs one statement without a screen and writes the result to a stream,
// so a script, a Makefile or a CI job uses the same profiles, timeouts and access limits as
// the client.
//
// Nothing here draws. The exit code is the answer for a caller that reads no output.
package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/result"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The exit codes. A caller that reads no output tells the four apart by these.
const (
	CodeOK = 0
	// CodeStatement is written when the server refused a statement, or it could not bind.
	CodeStatement  = 1
	CodeConnection = 2
	CodeRefused    = 3
)

// connectTimeout is how long the server has to accept the connection.
const connectTimeout = 30 * time.Second

// Format is how a result is written.
type Format string

const (
	FormatTable    Format = "table"
	FormatCSV      Format = "csv"
	FormatJSON     Format = "json"
	FormatMarkdown Format = "markdown"
)

// Formats lists the formats, the default first.
var Formats = []Format{FormatTable, FormatCSV, FormatJSON, FormatMarkdown}

// FormatNames returns the format names for use in an error message.
func FormatNames() string {
	names := make([]string, 0, len(Formats))
	for _, held := range Formats {
		names = append(names, string(held))
	}
	return strings.Join(names, ", ")
}

// FindFormat parses the text as a format.
func FindFormat(written string) (Format, bool) {
	return core.FindAllowed(Formats, strings.ToLower(strings.TrimSpace(written)))
}

// Options is what one run was asked to do.
type Options struct {
	Profile  cfg.Profile
	Password string
	// The statements to run, as one text.
	Statement string
	Format    Format
	// How many rows one statement returns. Zero reads every row the statement returns.
	RowLimit int
	Params   map[string]any
	Explain  bool
	Out      io.Writer
	Err      io.Writer
}

// report writes one line to the error stream.
func (options Options) report(format string, parts ...any) {
	_, _ = fmt.Fprintf(options.Err, "masume: "+format+"\n", parts...)
}

// Run opens the connection, runs every statement, and returns the exit code of the run.
func Run(ctx context.Context, adapters engines.Adapters, options Options) int {
	if cfg.NeedsPasswordPrompt(options.Profile) && options.Password == "" {
		options.report("%s needs a password, and a run without a screen cannot ask for one; "+
			"set password_env or password_command on the profile", options.Profile.Name)
		return CodeConnection
	}

	session, preConnect, code := openSession(ctx, adapters, options)
	if code != CodeOK {
		return code
	}
	defer func() {
		_ = session.Close()
		preConnect.Stop()
	}()

	held := session.Language()
	statements := held.SplitStatements(options.Statement)
	if len(statements) == 0 {
		options.report("there is no statement to run")
		return CodeStatement
	}

	if options.Format == FormatJSON && len(statements) > 1 {
		options.report("json holds one result, and this run has %d statements; "+
			"use csv or table, or run them one at a time", len(statements))
		return CodeStatement
	}

	for _, sql := range statements {
		if code := runOneStatement(ctx, session, held, options, sql); code != CodeOK {
			return code
		}
	}
	return CodeOK
}

// resolveRowLimit returns how many rows one read returns: the number the caller asked for,
// or one page of the profile.
func resolveRowLimit(options Options) int {
	if options.RowLimit > 0 {
		return options.RowLimit
	}
	return resolveBatchSize(options)
}

// resolveBatchSize returns how many rows a stream hands over at a time.
func resolveBatchSize(options Options) int {
	if options.Profile.PageSize < 1 {
		return cfg.DefaultPageSize
	}
	return options.Profile.PageSize
}

// openSession runs the pre-connect command of the profile and opens the connection.
func openSession(
	ctx context.Context, adapters engines.Adapters, options Options,
) (db.Session, *cfg.PreConnectHandle, int) {
	preConnect, err := cfg.StartPreConnectCommand(options.Profile)
	if err != nil {
		options.report("%s", err)
		return nil, nil, CodeConnection
	}

	opening, stop := context.WithTimeout(ctx, connectTimeout)
	defer stop()

	session, err := adapters.Open(opening, options.Profile, options.Password)
	if err != nil {
		preConnect.Stop()
		options.report("%s", db.DescribeError(err))
		return nil, nil, CodeConnection
	}
	return session, preConnect, CodeOK
}

// runOneStatement binds, refuses or runs one statement and writes its result.
func runOneStatement(
	ctx context.Context, session db.Session, held language.Language,
	options Options, sql string,
) int {
	writes := held.ResolveWriteRisk(sql) != statement.RiskNone
	if options.Profile.AccessMode == cfg.AccessReadOnly && writes {
		options.report("%s is read-only, so the statement was not sent", options.Profile.Name)
		return CodeRefused
	}

	if options.Explain {
		return writePlan(ctx, session, held, options, sql)
	}

	bound, err := session.Composer().BindParameters(sql, options.Params)
	if err != nil {
		options.report("%s", err)
		return CodeStatement
	}

	// A statement that bounds its own result is read whole. Any other is read one page at
	// a time, the same as in the client.
	if writes || options.RowLimit > 0 || !held.HoldsRowLimit(sql) {
		return writeOneRead(ctx, session, options, bound, writes)
	}
	return streamWholeRead(ctx, session, options, bound)
}

// writeOneRead runs the statement one time and writes what came back.
func writeOneRead(
	ctx context.Context, session db.Session, options Options, bound db.BoundText, writes bool,
) int {
	rowLimit := resolveRowLimit(options)
	answered, err := session.RunQuery(ctx, bound.Text, rowLimit, bound.Params)
	if err != nil {
		options.report("%s", db.DescribeError(err))
		return CodeStatement
	}
	if len(answered.Columns) == 0 {
		// The output stream holds the document alone, so this goes to the error stream.
		options.report("%s", describeChange(answered))
		return CodeOK
	}

	sink := createRowSink(options.Format, options.Out)
	if len(answered.Rows) > 0 {
		if err := sink.TakeRows(answered.Rows, answered.Columns); err != nil {
			options.report("the result could not be written: %v", err)
			return CodeStatement
		}
	}
	if err := sink.Finish(answered.Columns); err != nil {
		options.report("the result could not be written: %v", err)
		return CodeStatement
	}

	if !answered.Truncated {
		return CodeOK
	}
	if options.RowLimit > 0 {
		options.report("the first %d rows of a longer result, which is the number asked for",
			len(answered.Rows))
		return CodeOK
	}
	if writes {
		options.report("only the first %d rows of a longer result: a statement that "+
			"changes something is never run twice", len(answered.Rows))
		return CodeStatement
	}
	options.report("the first %d rows of a longer result; add a limit to the statement, "+
		"or --limit, to read more", len(answered.Rows))
	return CodeOK
}

// streamWholeRead reads every row the statement returns, a batch at a time.
func streamWholeRead(
	ctx context.Context, session db.Session, options Options, bound db.BoundText,
) int {
	sink := createRowSink(options.Format, options.Out)
	columns := []query.ResultColumn{}

	_, err := session.StreamQuery(ctx, bound.Text, bound.Params, resolveBatchSize(options),
		func(rows [][]any, held []query.ResultColumn) error {
			columns = held
			return sink.TakeRows(rows, held)
		})
	if err != nil {
		options.report("%s", db.DescribeError(err))
		return CodeStatement
	}

	// The statement is not read again for the columns: one this client reads as changing
	// nothing can still take a lock, `select … for update` among them.

	if err := sink.Finish(columns); err != nil {
		options.report("the result could not be written: %v", err)
		return CodeStatement
	}
	return CodeOK
}

// writePlan writes the plan of the statement as JSON.
func writePlan(
	ctx context.Context, session db.Session, held language.Language, options Options, sql string,
) int {
	if !session.Capabilities().PlansStatement {
		options.report("%s", db.DescribeError(db.NewUnsupportedError("plan a statement")))
		return CodeStatement
	}
	if !session.Capabilities().PlansEveryStatement && !held.CanExplain(sql) {
		options.report("the server has no plan for this statement")
		return CodeStatement
	}

	// A server does not plan a statement that still holds a placeholder.
	shown, err := statement.InlineQueryParameters(sql, options.Params, session.Dialect())
	if err != nil {
		options.report("%s", err)
		return CodeStatement
	}

	// Measuring a write would run it.
	analyze := session.Capabilities().MeasuresPlan && held.ResolveWriteRisk(sql) == statement.RiskNone
	plan, err := session.ExplainQuery(ctx, shown, analyze)
	if err != nil {
		options.report("%s", db.DescribeError(err))
		return CodeStatement
	}

	nodes := []map[string]any{}
	for _, row := range result.FlattenPlan(plan) {
		nodes = append(nodes, describePlanNode(row))
	}
	return writeJSON(options, map[string]any{
		"analyzed": plan.Analyzed,
		"summary":  result.DescribePlanCost(plan),
		"nodes":    nodes,
	})
}

// describePlanNode returns one node of a plan. A count the server did not measure or
// estimate is written as null, not as zero.
func describePlanNode(row result.PlanRow) map[string]any {
	var estimatedRows, actualRows, selfMs any
	if row.Node.HasEstimatedRows {
		estimatedRows = row.Node.EstimatedRows
	}
	if row.Node.HasActualRows {
		actualRows = row.Node.ActualRows
	}
	if row.Node.HasSelfMs {
		selfMs = row.Node.SelfMs
	}
	return map[string]any{
		"depth": row.Depth, "label": row.Node.Label, "detail": row.Node.Detail,
		"estimatedRows": estimatedRows, "actualRows": actualRows, "selfMs": selfMs,
		"shareOfTotal": row.Share, "slowest": row.Slowest,
		"misestimated": row.Misestimated,
	}
}

func writeJSON(options Options, written any) int {
	encoded, err := json.MarshalIndent(written, "", "  ")
	if err != nil {
		options.report("the answer could not be written as JSON: %v", err)
		return CodeStatement
	}
	// A closed pipe would otherwise tell a script its output is whole when it is not.
	if _, err := fmt.Fprintln(options.Out, string(encoded)); err != nil {
		options.report("the answer could not be written: %v", err)
		return CodeStatement
	}
	return CodeOK
}

// describeChange returns what a statement without a result set did.
func describeChange(answered db.QueryResult) string {
	command := answered.Command
	if command == "" {
		command = "the statement ran"
	}
	if !answered.HasAffected {
		return command
	}
	return fmt.Sprintf("%s %d", command, answered.Affected)
}
