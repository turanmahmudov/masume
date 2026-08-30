package redis

import (
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// A key is grouped in the tree by its prefix, which is the part before the first separator.
func TestReadKeyPrefixTakesThePartBeforeTheSeparator(t *testing.T) {
	for _, held := range []struct {
		key  string
		want string
	}{
		{"order:1", "order"},
		{"order:1:line:2", "order"},
		{"plain", "plain"},
		{"", ""},
		// A key that opens with the separator holds an empty prefix, which is still a group.
		{":held", ""},
	} {
		if answered := ReadKeyPrefix(held.key); answered != held.want {
			t.Errorf("%q groups under %q, wanted %q", held.key, answered, held.want)
		}
	}
}

// A key belongs to a prefix only where the whole prefix matches up to a separator, or
// `order` would claim `orders:1` and the tree would show the wrong keys.
func TestMatchesPrefixDoesNotClaimALongerName(t *testing.T) {
	for _, held := range []struct {
		key    string
		prefix string
		want   bool
	}{
		{"order:1", "order", true},
		{"order", "order", true},
		{"order:1:line:2", "order", true},

		{"orders:1", "order", false},
		{"ordering", "order", false},
		{"other:1", "order", false},
		{"", "order", false},
	} {
		if answered := MatchesPrefix(held.key, held.prefix); answered != held.want {
			t.Errorf("%q under %q = %v, wanted %v",
				held.key, held.prefix, answered, held.want)
		}
	}
}

// A prefix goes into a SCAN pattern, where some characters mean something. A key holding one
// of them must match itself and not a pattern.
func TestBuildMatchPatternEscapesWhatAPatternWouldReadAsAMark(t *testing.T) {
	for _, held := range []struct {
		prefix string
		holds  string
	}{
		{"order", "order:*"},
		{"a*b", `a\*b:*`},
		{"a?b", `a\?b:*`},
		{"a[b", `a\[b:*`},
		{`a\b`, `a\\b:*`},
	} {
		if answered := BuildMatchPattern(held.prefix); answered != held.holds {
			t.Errorf("%q becomes %q, wanted %q", held.prefix, answered, held.holds)
		}
	}
}

// Every value a driver hands over has to draw as text in one cell of the grid.
func TestFormatRedisValueWritesEveryShapeAsText(t *testing.T) {
	for _, held := range []struct {
		name  string
		value any
		want  string
	}{
		{"nothing", nil, ""},
		{"text", "ada", "ada"},
		{"bytes", []byte("ada"), "ada"},
		{"a number", int64(42), "42"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := FormatRedisValue(held.value); answered != held.want {
				t.Errorf("%v writes as %q, wanted %q", held.value, answered, held.want)
			}
		})
	}

	// A container becomes JSON, so a list or a hash reads in one cell.
	written := FormatRedisValue([]any{"a", "b"})
	if !strings.Contains(written, "a") || !strings.Contains(written, "b") {
		t.Errorf("a list writes as %q", written)
	}
}

// buildKeyTarget answers a staged write against the key columns of the grid.
func buildKeyTarget(rows [][]any) db.ChangeTarget {
	columns := make([]db.ResultColumn, 0, len(KeyColumnNames))
	for _, name := range KeyColumnNames {
		columns = append(columns, db.ResultColumn{Name: name})
	}
	return db.ChangeTarget{
		Columns:    columns,
		Rows:       rows,
		KeyColumns: []string{KeyColumnKey},
	}
}

// An edit to the value of a key becomes the command that sets it, and the key of the row is
// what the command names.
func TestBuildChangesWritesTheCommandForAnEdit(t *testing.T) {
	valueAt := -1
	for at, name := range KeyColumnNames {
		if name == KeyColumnValue {
			valueAt = at
		}
	}
	if valueAt < 0 {
		t.Fatal("the key columns hold no value column")
	}

	target := buildKeyTarget([][]any{{"order:1", "string", int64(-1), "ada"}})
	staged := core.NewPendingChanges()
	staged.Edits[core.BuildEditKey(0, valueAt)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: valueAt,
		Value: core.CellValue{Kind: core.CellText, Text: "grace"},
	}

	changes, err := BuildChanges(target, staged)
	if err != nil {
		t.Fatalf("the changes answered %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("the staged work became %d changes, wanted 1", len(changes))
	}
	if !strings.Contains(changes[0].Display, "order:1") {
		t.Errorf("the command reads %q and does not name the key", changes[0].Display)
	}
	if changes[0].Description == "" {
		t.Error("the change carries no description for the review card")
	}
}

// A row marked for deletion becomes the command that removes the key, and skips its own edits
// because the key is going.
func TestBuildChangesRemovesADeletedKeyAndSkipsItsEdits(t *testing.T) {
	target := buildKeyTarget([][]any{{"order:1", "string", int64(-1), "ada"}})
	staged := core.NewPendingChanges()
	staged.DeletedRows[0] = true
	staged.Edits[core.BuildEditKey(0, 3)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 3,
		Value: core.CellValue{Kind: core.CellText, Text: "grace"},
	}

	changes, err := BuildChanges(target, staged)
	if err != nil {
		t.Fatalf("the changes answered %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("the staged work became %d changes, wanted the delete alone", len(changes))
	}
	if !strings.Contains(strings.ToUpper(changes[0].Display), "DEL") {
		t.Errorf("the command reads %q, wanted one that removes the key", changes[0].Display)
	}
}

func TestBuildChangesAnswersNothingForNoStagedWork(t *testing.T) {
	changes, err := BuildChanges(buildKeyTarget(nil), core.NewPendingChanges())
	if err != nil {
		t.Fatalf("no staged work answered %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("no staged work became %d changes", len(changes))
	}
}

// A reply of any shape becomes the columns and rows of a result, because the grid draws every
// answer the same way.
func TestBuildReplyResultReadsEveryShapeOfReply(t *testing.T) {
	for _, held := range []struct {
		name  string
		reply any
	}{
		{"nothing", nil},
		{"one value", "ada"},
		{"a list", []any{"ada", "grace"}},
		{"a number", int64(3)},
		{"a hash", map[string]any{"customer": "ada"}},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered := BuildReplyResult(held.reply, time.Millisecond)
			// Every reply has to draw, so a result with rows needs columns to draw them in.
			if len(answered.Rows) > 0 && len(answered.Columns) == 0 {
				t.Errorf("%v gave %d rows and no column", held.reply, len(answered.Rows))
			}
			for _, row := range answered.Rows {
				if len(row) != len(answered.Columns) {
					t.Errorf("a row holds %d values and there are %d columns",
						len(row), len(answered.Columns))
				}
			}
		})
	}
}

// The activity list is parsed out of the lines the server prints, and a line it cannot read
// must be dropped rather than shown as an empty session.
func TestReadClientLineReadsASessionAndRefusesTheRest(t *testing.T) {
	const line = "id=7 addr=127.0.0.1:6379 name=masume age=12 idle=0 cmd=get user=default"
	held, is := ReadClientLine(line)
	if !is {
		t.Fatal("a line of the server was not read")
	}
	if held.PID != 7 {
		t.Errorf("the session is numbered %d, wanted 7", held.PID)
	}
	if held.ClientAddress == "" {
		t.Error("the session carries no address")
	}

	for _, written := range []string{"", "   ", "nothing useful"} {
		if _, is := ReadClientLine(written); is {
			t.Errorf("%q was read as a session", written)
		}
	}
}

// EXPIRE takes whole seconds, and it removes the key where the number is not above zero.
// A part of a second rounds down to that, so a TTL written as `0.5` would delete the key
// the reader meant to keep.
func TestBuildTTLCommandTakesAWholeNumberOfSecondsOnly(t *testing.T) {
	for _, held := range []struct {
		written  string
		wantName string
		wantArg  string
		wantFail bool
	}{
		{"60", "EXPIRE", "60", false},
		{"1", "EXPIRE", "1", false},

		// Every one of these would have reached EXPIRE with a zero, or with a number
		// the conversion does not define.
		{"0.5", "", "", true},
		{"0.999", "", "", true},
		{"0", "", "", true},
		{"-1", "", "", true},
		{"NaN", "", "", true},
		{"Inf", "", "", true},
		{"nonsense", "", "", true},
	} {
		t.Run(held.written, func(t *testing.T) {
			command, err := buildTTLCommand("order:1", held.written, false)
			if held.wantFail {
				if err == nil {
					t.Fatalf("%q was taken as a ttl, and built %v", held.written, command)
				}
				return
			}
			if err != nil {
				t.Fatalf("%q was refused: %v", held.written, err)
			}
			if command.name != held.wantName || command.args[1] != held.wantArg {
				t.Errorf("%q built %s %v, wanted %s %s",
					held.written, command.name, command.args, held.wantName, held.wantArg)
			}
		})
	}

	// An empty value clears the TTL rather than removing the key.
	cleared, err := buildTTLCommand("order:1", "", false)
	if err != nil || cleared.name != "PERSIST" {
		t.Errorf("an empty ttl built %s, %v", cleared.name, err)
	}
}
