// Package headless runs statements without a screen, writes results to a stream, and returns an exit code.
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

// Process exit codes.
const (
	CodeOK = 0
	// CodeStatement is a statement, parameter, plan, or output failure.
	CodeStatement  = 1
	CodeConnection = 2
	CodeRefused    = 3
)

// connectTimeout is the connection timeout.
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

// Options is a headless request.
type Options struct {
	Profile  cfg.Profile
	Password string
	// The statements to run, as one text.
	Statement string
	Format    Format
	// Row limit. Zero uses the profile page size unless the statement has a limit.
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
		options.report("%s requires a password; headless mode cannot prompt for passwords. "+
			"Set password_env, password_command, or a [secret] store on the profile, or "+
			"save the password in the keyring through the interactive client",
			options.Profile.Name)
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
		options.report("no statement to run")
		return CodeStatement
	}

	if options.Format == FormatJSON && len(statements) > 1 {
		options.report("json output supports one statement per run; received %d statements. "+
			"Use csv or table, or run each statement separately", len(statements))
		return CodeStatement
	}

	for _, sql := range statements {
		if code := runOneStatement(ctx, session, held, options, sql); code != CodeOK {
			return code
		}
	}
	return CodeOK
}

// resolveRowLimit returns the requested row limit or the profile page size.
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

	// Stream queries with their own limit unless an explicit output limit applies.
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
	if len(answered.Columns) == 0 && !answered.HoldsResultSet {
		// Write statement status to stderr.
		options.report("%s", describeChange(answered))
		return CodeOK
	}

	sink := createRowSink(options.Format, options.Out)
	if len(answered.Rows) > 0 {
		if err := sink.TakeRows(answered.Rows, answered.Columns); err != nil {
			options.report("cannot write the result: %v", err)
			return CodeStatement
		}
	}
	if err := sink.Finish(answered.Columns); err != nil {
		options.report("cannot write the result: %v", err)
		return CodeStatement
	}

	if !answered.Truncated {
		return CodeOK
	}
	if options.RowLimit > 0 {
		options.report("returned the first %d rows; the result exceeds the requested limit",
			len(answered.Rows))
		return CodeOK
	}
	if writes {
		options.report("returned only the first %d rows; write results are incomplete. "+
			"The write was not repeated. Do not automatically retry the write", len(answered.Rows))
		return CodeStatement
	}
	options.report("returned the first %d rows; the result exceeds the page size. Add a statement limit "+
		"or --limit to read more", len(answered.Rows))
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

	// Do not repeat the query for column metadata. SELECT FOR UPDATE can acquire locks.

	if err := sink.Finish(columns); err != nil {
		options.report("cannot write the result: %v", err)
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
		options.report("the server does not support plans for this statement")
		return CodeStatement
	}

	// Replace named parameters before requesting a plan.
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

// describePlanNode returns a plan node with null for unavailable counts and times.
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
		options.report("cannot encode the result as JSON: %v", err)
		return CodeStatement
	}
	// Output failures return a nonzero exit code.
	if _, err := fmt.Fprintln(options.Out, string(encoded)); err != nil {
		options.report("cannot write the result: %v", err)
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
