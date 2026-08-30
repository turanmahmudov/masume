package redis

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// Nothing here reaches the server. This holds the shape of the key browser and builds
// what to send, and the adapter sends it.

// ScanBatch is how many keys one SCAN asks for. The server can return fewer.
const ScanBatch = 500

// The columns of the key browser, in the order a row holds them.
const (
	KeyColumnKey   = "key"
	KeyColumnType  = "type"
	KeyColumnTTL   = "ttl"
	KeyColumnValue = "value"
)

// KeyColumnNames lists the columns of the key browser.
var KeyColumnNames = []string{KeyColumnKey, KeyColumnType, KeyColumnTTL, KeyColumnValue}

func resolveKeyColumnIndex(name string) int {
	for at, held := range KeyColumnNames {
		if held == name {
			return at
		}
	}
	return -1
}

// KeyColumns are the columns the key browser shows, the same for every prefix.
var KeyColumns = []db.ColumnDetail{
	{Name: KeyColumnKey, DataType: "key", IsPrimaryKey: true},
	{
		Name: KeyColumnType, DataType: "type", IsGenerated: true,
		Choices: []string{"string", "list", "set", "zset", "hash", "stream"},
	},
	{Name: KeyColumnTTL, DataType: "seconds", Nullable: true},
	{Name: KeyColumnValue, DataType: "value", Nullable: true},
}

// KeyResultColumns are the same columns, as a result carries them.
var KeyResultColumns = func() []db.ResultColumn {
	columns := make([]db.ResultColumn, 0, len(KeyColumns))
	for _, column := range KeyColumns {
		columns = append(columns, db.ResultColumn{Name: column.Name, DataType: column.DataType})
	}
	return columns
}()

// KeyRead is a relation as a key prefix.
type KeyRead struct {
	Prefix string
	// Kept as steps, so the engine tests each key itself.
	Filter []core.FilterStep
}

// RedisCommand is a command to run.
type RedisCommand struct {
	Name string
	Args []string
}

// ReadRedisCommand reads back the command a change carries. A change built by another
// engine is refused, not sent as a command.
func ReadRedisCommand(change db.Change) (RedisCommand, error) {
	command, built := change.Payload.(RedisCommand)
	if !built {
		return RedisCommand{}, core.NewEditError("this change was not built for this session")
	}
	return command, nil
}

// FindKeyRead returns the key read this engine composed, or nothing for another payload.
func FindKeyRead(read db.ComposedRead) (KeyRead, bool) {
	held, composed := read.Payload.(KeyRead)
	return held, composed
}

// BuildMatchPattern writes a pattern that matches every key of one prefix, with the
// pattern marks escaped.
func BuildMatchPattern(prefix string) string {
	var escaped strings.Builder
	for _, character := range prefix {
		if strings.ContainsRune(`[]?*\`, character) {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(character)
	}
	return escaped.String() + ":*"
}

// FormatRedisValue writes one value as cell text. A container becomes JSON, and a
// string stays as it is.
func FormatRedisValue(value any) string {
	switch held := value.(type) {
	case nil:
		return ""
	case string:
		return held
	case []byte:
		return string(held)
	case int64:
		return strconv.FormatInt(held, 10)
	case int:
		return strconv.Itoa(held)
	case float64:
		return strconv.FormatFloat(held, 'g', -1, 64)
	}
	written, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(written)
}

// stagedCommand is one command the grid staged, before it is written as a change.
type stagedCommand struct {
	name        string
	args        []string
	description string
}

func buildRedisChange(command stagedCommand) db.Change {
	written := make([]string, 0, len(command.args)+1)
	written = append(written, command.name)
	for _, argument := range command.args {
		if strings.Contains(argument, " ") {
			written = append(written, `"`+argument+`"`)
			continue
		}
		written = append(written, argument)
	}
	return db.Change{
		Description: command.description,
		Display:     strings.Join(written, " "),
		Payload:     RedisCommand{Name: command.name, Args: command.args},
	}
}

func buildInsertCommand(values map[string]any) (stagedCommand, error) {
	key := strings.TrimSpace(FormatRedisValue(values[KeyColumnKey]))
	if key == "" {
		return stagedCommand{}, core.NewEditError("a new key needs a name")
	}
	return stagedCommand{
		name: "SET", args: []string{key, FormatRedisValue(values[KeyColumnValue])},
		description: "set " + key,
	}, nil
}

func buildValueCommand(key, keyType, written string) (stagedCommand, error) {
	if keyType != "string" {
		return stagedCommand{}, core.NewEditError(fmt.Sprintf(
			"a %s is edited through its own commands, not as one value", keyType))
	}
	return stagedCommand{name: "SET", args: []string{key, written}, description: "set " + key}, nil
}

func buildTTLCommand(key, written string, cleared bool) (stagedCommand, error) {
	if cleared || strings.TrimSpace(written) == "" {
		return stagedCommand{
			name: "PERSIST", args: []string{key}, description: "clear the ttl of " + key,
		}, nil
	}
	// EXPIRE takes whole seconds, and it removes the key where the number is not above
	// zero. A part of a second would round down to that, so a TTL of `0.5` would delete
	// the key the user meant to keep, and the number is read as a whole one here.
	seconds, err := strconv.ParseInt(strings.TrimSpace(written), 10, 64)
	if err != nil || seconds <= 0 {
		return stagedCommand{}, core.NewEditError(
			"a ttl is a whole number of seconds above nothing")
	}
	return stagedCommand{
		name: "EXPIRE", args: []string{key, strconv.FormatInt(seconds, 10)},
		description: fmt.Sprintf("expire %s in %ds", key, seconds),
	}, nil
}

// buildEditCommand returns the one command a staged cell asks for, by the column it
// sits in.
func buildEditCommand(
	edit core.CellEdit, column, key, keyType string,
) (stagedCommand, error) {
	written := ""
	if edit.Value.Kind == core.CellText {
		written = edit.Value.Text
	}
	switch column {
	case KeyColumnValue:
		return buildValueCommand(key, keyType, written)
	case KeyColumnTTL:
		return buildTTLCommand(key, written, edit.Value.Kind == core.CellNull)
	}
	return stagedCommand{}, core.NewEditError(column + " is not something a write can change")
}

func findDeleteCommand(keys []string) (stagedCommand, bool) {
	named := make([]string, 0, len(keys))
	for _, key := range keys {
		if key != "" {
			named = append(named, key)
		}
	}
	if len(named) == 0 {
		return stagedCommand{}, false
	}
	description := fmt.Sprintf("delete %d keys", len(named))
	if len(named) == 1 {
		description = "delete " + named[0]
	}
	return stagedCommand{name: "DEL", args: named, description: description}, true
}

// BuildChanges returns the commands that apply what the grid staged in a key
// browser.
func BuildChanges(target db.ChangeTarget, staged core.PendingChanges) ([]db.Change, error) {
	readCell := func(rowIndex int, name string) string {
		at := resolveKeyColumnIndex(name)
		if rowIndex < 0 || rowIndex >= len(target.Rows) || at < 0 || at >= len(target.Rows[rowIndex]) {
			return ""
		}
		return FormatRedisValue(target.Rows[rowIndex][at])
	}

	commands := []stagedCommand{}
	for _, values := range staged.Inserts {
		command, err := buildInsertCommand(values)
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	}

	for _, edit := range core.SortedEdits(staged) {
		if staged.DeletedRows[edit.RowIndex] {
			continue
		}
		key := readCell(edit.RowIndex, KeyColumnKey)
		if key == "" {
			continue
		}
		column := ""
		if edit.ColumnIndex >= 0 && edit.ColumnIndex < len(target.Columns) {
			column = target.Columns[edit.ColumnIndex].Name
		}
		command, err := buildEditCommand(edit, column, key, readCell(edit.RowIndex, KeyColumnType))
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	}

	deleted := core.SortedDeletedRows(staged)
	keys := make([]string, 0, len(deleted))
	for _, rowIndex := range deleted {
		keys = append(keys, readCell(rowIndex, KeyColumnKey))
	}
	if command, found := findDeleteCommand(keys); found {
		commands = append(commands, command)
	}

	changes := make([]db.Change, 0, len(commands))
	for _, command := range commands {
		changes = append(changes, buildRedisChange(command))
	}
	return changes, nil
}

// ComposeRelationRead returns the scan of one key prefix.
func ComposeRelationRead(table db.TableRef, rewrite core.ReadRewrite) db.ComposedRead {
	text := fmt.Sprintf("SCAN 0 MATCH %s COUNT %d", BuildMatchPattern(table.Name), ScanBatch)
	return db.ComposedRead{
		Text: text, Display: text, Pageable: true,
		Payload: KeyRead{Prefix: table.Name, Filter: rewrite.Filter},
	}
}

// ComposeStatementRead returns a command of the user, which runs as it is with
// nothing laid over it.
func ComposeStatementRead(written db.BoundText) db.ComposedRead {
	return db.ComposedRead{Text: written.Text, Params: written.Params, Display: written.Text}
}

// ReadKeyPrefix returns the prefix of a key: the part before the first separator.
func ReadKeyPrefix(key string) string {
	before, _, ok := strings.Cut(key, ":")
	if !ok {
		return key
	}
	return before
}

// MatchesPrefix is true if the key belongs to this prefix.
func MatchesPrefix(key, prefix string) bool {
	return key == prefix || strings.HasPrefix(key, prefix+":")
}

// BuildReplyResult turns a reply of any shape into the columns and rows of a result.
func BuildReplyResult(reply any, elapsed time.Duration) db.QueryResult {
	switch held := reply.(type) {
	case nil:
		return db.QueryResult{
			Columns: []db.ResultColumn{{Name: "reply", DataType: "nil"}},
			Rows:    [][]any{{nil}}, Elapsed: elapsed,
		}
	case []any:
		rows := make([][]any, 0, len(held))
		for _, entry := range held {
			rows = append(rows, []any{FormatRedisValue(entry)})
		}
		return db.QueryResult{
			Columns: []db.ResultColumn{{Name: "reply", DataType: "array"}},
			Rows:    rows, Elapsed: elapsed,
		}
	case map[string]any:
		fields := make([]string, 0, len(held))
		for field := range held {
			fields = append(fields, field)
		}
		slices.Sort(fields)
		rows := make([][]any, 0, len(held))
		for _, field := range fields {
			rows = append(rows, []any{field, FormatRedisValue(held[field])})
		}
		return db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "field", DataType: "field"}, {Name: "value", DataType: "value"},
			},
			Rows: rows, Elapsed: elapsed,
		}
	case int64:
		return db.QueryResult{
			Columns: []db.ResultColumn{{Name: "reply", DataType: "integer"}},
			Rows:    [][]any{{held}}, Elapsed: elapsed,
		}
	}
	return db.QueryResult{
		Columns: []db.ResultColumn{{Name: "reply", DataType: "string"}},
		Rows:    [][]any{{FormatRedisValue(reply)}}, Elapsed: elapsed,
	}
}

// ReadClientLine reads one line of CLIENT LIST, which is `name=value` pairs separated
// by spaces.
func ReadClientLine(line string) (db.Activity, bool) {
	fields := map[string]string{}
	for pair := range strings.FieldsSeq(strings.TrimSpace(line)) {
		at := strings.Index(pair, "=")
		if at > 0 {
			fields[pair[:at]] = pair[at+1:]
		}
	}
	id, err := strconv.ParseInt(fields["id"], 10, 64)
	if err != nil {
		return db.Activity{}, false
	}
	age, _ := strconv.ParseFloat(fields["age"], 64)

	return db.Activity{
		PID: id, User: fields["user"], ApplicationName: fields["name"],
		ClientAddress: fields["addr"], State: fields["cmd"],
		Duration: time.Duration(age * float64(time.Second)), Query: fields["cmd"],
	}, true
}

// readStepValue returns the value of a step, as the text a key or a cell is compared
// with.
func readStepValue(step core.FilterStep) string {
	if step.Kind == core.FilterRaw {
		return step.Text
	}
	return FormatRedisValue(step.Value)
}

// matchesStep tests the rows already read, not the server. Redis matches a key by
// pattern only, so a step on the type or the value is applied here.
func matchesStep(row []any, step core.FilterStep) bool {
	if step.Kind == core.FilterRaw {
		// The text the user typed is read as a pattern over the key.
		at := resolveKeyColumnIndex(KeyColumnKey)
		key := ""
		if at >= 0 && at < len(row) {
			key = FormatRedisValue(row[at])
		}
		return strings.Contains(key, strings.ReplaceAll(step.Text, "*", ""))
	}

	at := -1
	for index, column := range KeyResultColumns {
		if column.Name == step.Column {
			at = index
			break
		}
	}
	if at == -1 || at >= len(row) {
		return true
	}
	value := row[at]
	written := ""
	if value != nil {
		written = FormatRedisValue(value)
	}
	switch step.Test {
	case core.FilterIsNull:
		return value == nil
	case core.FilterIsNotNull:
		return value != nil
	case core.FilterEquals:
		return written == readStepValue(step)
	}
	return written != readStepValue(step)
}

// KeepsRow is true where the row passes every step of the filter.
func KeepsRow(row []any, filter []core.FilterStep) bool {
	for _, step := range filter {
		if !matchesStep(row, step) {
			return false
		}
	}
	return true
}
