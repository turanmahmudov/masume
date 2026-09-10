package headless

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/dump"
	"github.com/turanmahmudov/masume/internal/present"
)

// `masume dump` writes a schema as SQL, and `masume restore` runs such a file back into a
// server. Neither has a confirmation, a write plan or an undo.

// DumpOptions is a headless dump request.
type DumpOptions struct {
	Options
	// Path is the file the dump is written to. A single hyphen writes to the output
	// stream.
	Path string
	Dump dump.Options
}

// RestoreOptions is a headless restore request.
type RestoreOptions struct {
	Options
	// Path is the file the statements are read from. A single hyphen reads the input
	// stream.
	Path string
	In   io.Reader
}

// stdoutPath is the path that means the stream rather than a file.
const stdoutPath = "-"

// RunDump opens the connection, writes the dump, and returns the exit code of the run.
func RunDump(ctx context.Context, adapters engines.Adapters, options DumpOptions) int {
	if code := checkPassword(options.Options); code != CodeOK {
		return code
	}
	session, preConnect, code := openSession(ctx, adapters, options.Options)
	if code != CodeOK {
		return code
	}
	defer func() {
		_ = session.Close()
		preConnect.Stop()
	}()

	if !session.Capabilities().WritesDDL {
		options.report("this server does not report definitions, so it cannot be dumped")
		return CodeStatement
	}
	if options.Dump.Schema == "" {
		options.Dump.Schema = session.Describe().DefaultSchema
	}
	if options.Dump.Schema == "" {
		options.report("this connection names no schema; name one with --schema")
		return CodeStatement
	}

	report, code := writeDumpFile(ctx, session, options)
	if code != CodeOK {
		return code
	}
	options.report("dumped %s", report.Describe())
	return CodeOK
}

// writeDumpFile writes the dump to the stream or to the file the options name.
func writeDumpFile(
	ctx context.Context, session db.Session, options DumpOptions,
) (dump.Report, int) {
	if options.Path == stdoutPath {
		report, err := dump.Write(ctx, session, options.Dump, options.Out)
		if err != nil {
			options.report("%s", db.DescribeError(err))
			return report, CodeStatement
		}
		return report, CodeOK
	}

	path := core.ExpandHomePath(options.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		options.report("%s", err)
		return dump.Report{}, CodeStatement
	}
	// Written beside the file and moved over it at the end, so a read that fails part way
	// leaves the file that was there whole.
	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.part")
	if err != nil {
		options.report("%s", err)
		return dump.Report{}, CodeStatement
	}
	temporaryPath := file.Name()
	drop := func() {
		_ = file.Close()
		_ = os.Remove(temporaryPath)
	}
	// The file holds the rows of the database, so it stays readable by its owner alone.
	if modeErr := file.Chmod(0o600); modeErr != nil {
		drop()
		options.report("%s", modeErr)
		return dump.Report{}, CodeStatement
	}

	report, writeErr := dump.Write(ctx, session, options.Dump, file)
	if writeErr != nil {
		drop()
		options.report("%s", db.DescribeError(writeErr))
		return report, CodeStatement
	}
	if closeErr := file.Close(); closeErr != nil {
		_ = os.Remove(temporaryPath)
		options.report("%s", closeErr)
		return report, CodeStatement
	}
	if renameErr := os.Rename(temporaryPath, path); renameErr != nil {
		_ = os.Remove(temporaryPath)
		options.report("%s", renameErr)
		return report, CodeStatement
	}
	return report, CodeOK
}

// RunRestore opens the connection, runs every statement of the file, and returns the exit
// code of the run. It stops at the first statement the server refused.
func RunRestore(ctx context.Context, adapters engines.Adapters, options RestoreOptions) int {
	if options.Profile.AccessMode == cfg.AccessReadOnly {
		options.report("%s is read-only, so the file was not run", options.Profile.Name)
		return CodeRefused
	}
	if code := checkPassword(options.Options); code != CodeOK {
		return code
	}

	reader, closeReader, code := openRestoreFile(options)
	if code != CodeOK {
		return code
	}
	defer closeReader()

	session, preConnect, code := openSession(ctx, adapters, options.Options)
	if code != CodeOK {
		return code
	}
	defer func() {
		_ = session.Close()
		preConnect.Stop()
	}()

	// A file of statements is SQL, and the splitting reads the SQL of this engine. The
	// engines that report no definition hold another language.
	if !session.Capabilities().WritesDDL {
		options.report("this server holds no SQL definitions, so a dump cannot be restored")
		return CodeStatement
	}

	report, err := dump.Run(ctx, session, reader, session.Dialect().Syntax)
	if err != nil {
		options.report("statement %d failed: %s", report.Statements+1, db.DescribeError(err))
		options.report("%s ran before it",
			present.FormatCountOf(int64(report.Statements), "statement", "statements"))
		return CodeStatement
	}
	options.report("ran %s",
		present.FormatCountOf(int64(report.Statements), "statement", "statements"))
	return CodeOK
}

// openRestoreFile opens the file, or takes the input stream for a single hyphen.
func openRestoreFile(options RestoreOptions) (io.Reader, func(), int) {
	if options.Path == stdoutPath {
		return options.In, func() {}, CodeOK
	}
	file, err := os.Open(core.ExpandHomePath(options.Path))
	if err != nil {
		options.report("%s", err)
		return nil, nil, CodeConnection
	}
	return file, func() { _ = file.Close() }, CodeOK
}
