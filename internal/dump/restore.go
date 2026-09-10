package dump

import (
	"bufio"
	"context"
	"io"
	"os"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// A restore runs the statements of a file one at a time and stops at the first failure. The
// caller decides what a statement runs inside.

// FileExtensions are the extensions of a file a restore reads.
var FileExtensions = []string{".sql"}

// readBuffer is the size of one read of the file.
const readBuffer = 64 * 1024

// Runner runs one statement of a dump file.
type Runner interface {
	RunQuery(ctx context.Context, sql string, rowLimit int, params []any) (db.QueryResult, error)
}

// RunReport is what a restore ran.
type RunReport struct {
	Statements int
}

// WalkStatements reads the file and hands over each statement, in the order the file holds
// them. WalkStatements returns nil after a complete read, or the read or callback error that
// stopped it.
func WalkStatements(
	reader io.Reader, flavour syntax.SyntaxFlavour, onStatement func(sql string) error,
) error {
	buffered := bufio.NewReaderSize(reader, readBuffer)
	pending := []byte{}
	// The buffer is read for statement ends only once it has grown to this size, and the
	// size doubles with a statement that is larger than it. A block body holds semicolons
	// of its own, and reading the whole buffer again at each one of them would cost the
	// square of its length.
	nextRead := readBuffer

	for {
		// A read stops at a semicolon, which no other byte of a UTF-8 rune can be, so a
		// read never cuts a character in two. The semicolons inside a literal, a comment
		// or a block are found again below and left in place.
		chunk, err := buffered.ReadString(';')
		pending = append(pending, chunk...)
		if len(pending) < nextRead && err == nil {
			continue
		}

		// The statements before the last complete one are handed over, and the text
		// after it waits for the next read.
		held := string(pending)
		cut := statement.FindLastStatementEnd(held, flavour)
		for _, sql := range statement.SplitStatements(held[:cut], flavour) {
			if callbackErr := onStatement(sql); callbackErr != nil {
				return callbackErr
			}
		}
		pending = pending[:copy(pending, pending[cut:])]
		nextRead = max(readBuffer, len(pending)*2)

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	// The last statement of a file that ends without a semicolon.
	for _, sql := range statement.SplitStatements(string(pending), flavour) {
		if err := onStatement(sql); err != nil {
			return err
		}
	}
	return nil
}

// Run runs every statement the reader holds. The report holds the statements that ran, both
// after a complete run and after a failure.
func Run(
	ctx context.Context, runner Runner, reader io.Reader, flavour syntax.SyntaxFlavour,
) (RunReport, error) {
	report := RunReport{}
	err := WalkStatements(reader, flavour, func(sql string) error {
		if _, runErr := runner.RunQuery(ctx, sql, 1, nil); runErr != nil {
			return runErr
		}
		report.Statements++
		return nil
	})
	return report, err
}

// RunFile runs every statement of the file.
func RunFile(
	ctx context.Context, runner Runner, path string, flavour syntax.SyntaxFlavour,
) (RunReport, error) {
	file, err := os.Open(core.ExpandHomePath(path))
	if err != nil {
		return RunReport{}, err
	}
	defer func() { _ = file.Close() }()
	return Run(ctx, runner, file, flavour)
}
