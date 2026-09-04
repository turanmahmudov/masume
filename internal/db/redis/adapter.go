package redis

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// droppedLog takes what the driver would otherwise print. The screen is drawn over the whole
// terminal, so a line written to it scrolls the frame.
type droppedLog struct{}

func (droppedLog) Printf(context.Context, string, ...any) {}

func init() {
	redis.SetLogger(droppedLog{})
}

// catalogKeyLimit is how many keys the catalog reads. A keyspace can be endless, and the
// first ten thousand keys show every prefix that matters.
const catalogKeyLimit = 10_000

// everyKey asks for every key, for a read the caller caps itself.
const everyKey = math.MaxInt32

// valuePreviewKeys is how much of a list or a sorted set one cell shows.
const valuePreviewKeys = 100

// ddlTypeSample is how many keys of a prefix are typed, to count the types.
const ddlTypeSample = 200

// readRedisCommandLine reads one line of the buffer as a command and its arguments.
func readRedisCommandLine(line string) (RedisCommand, bool) {
	words := ReadRedisWords(line, 0)
	if len(words) == 0 {
		return RedisCommand{}, false
	}
	args := make([]string, 0, len(words)-1)
	for _, word := range words[1:] {
		args = append(args, word.Text)
	}
	return RedisCommand{Name: strings.ToUpper(words[0].Text), Args: args}, true
}

// redisSession is one session on a Redis server.
type redisSession struct {
	db.PlainCatalog
	db.NoUserTransactions
	db.NoServerLoad
	db.SessionFacts

	client *redis.Client
}

func (session *redisSession) send(ctx context.Context, command RedisCommand) (any, error) {
	args := make([]any, 0, len(command.Args)+1)
	args = append(args, command.Name)
	for _, argument := range command.Args {
		args = append(args, argument)
	}
	return session.client.Do(ctx, args...).Result()
}

// scanKeys returns every key the server holds, up to the cap, narrowed by a pattern.
func (session *redisSession) scanKeys(
	ctx context.Context, pattern string, wanted int,
) ([]string, error) {
	keys := []string{}
	cursor := uint64(0)

	for {
		found, next, err := session.client.Scan(ctx, cursor, pattern, ScanBatch).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, found...)
		cursor = next
		if cursor == 0 || len(keys) >= wanted {
			break
		}
	}
	return keys, nil
}

// readPreview returns the head of a list or a sorted set, as much as one cell shows.
func (session *redisSession) readPreview(
	ctx context.Context, command, key string, extra ...string,
) (any, error) {
	args := []string{key, "0", strconv.Itoa(valuePreviewKeys - 1)}
	return session.send(ctx, RedisCommand{Name: command, Args: append(args, extra...)})
}

// unreadableCell is drawn where the server refused a read. An empty cell would read as
// an empty value, and a key the server could not answer for is not an empty key.
const unreadableCell = "(unreadable)"

// readKeyValue returns the value of a key, read by its type.
func (session *redisSession) readKeyValue(
	ctx context.Context, key, keyType string,
) string {
	var value any
	var err error

	switch keyType {
	case "string":
		value, err = session.send(ctx, RedisCommand{Name: "GET", Args: []string{key}})
	case "list":
		value, err = session.readPreview(ctx, "LRANGE", key)
	case "set":
		value, err = session.send(ctx, RedisCommand{Name: "SMEMBERS", Args: []string{key}})
	case "zset":
		value, err = session.readPreview(ctx, "ZRANGE", key, "WITHSCORES")
	case "hash":
		value, err = session.send(ctx, RedisCommand{Name: "HGETALL", Args: []string{key}})
	default:
		return "(" + keyType + ")"
	}
	if err != nil {
		return unreadableCell
	}
	return FormatRedisValue(value)
}

// readKeyRow returns one key as a row: its type, its time to live, and its value.
func (session *redisSession) readKeyRow(ctx context.Context, key string) []any {
	keyType, typeErr := session.client.Type(ctx, key).Result()
	if typeErr != nil {
		return []any{key, unreadableCell, unreadableCell, unreadableCell}
	}
	ttl, ttlErr := session.client.TTL(ctx, key).Result()

	var seconds any
	switch {
	case ttlErr != nil:
		seconds = unreadableCell
	case ttl >= 0:
		seconds = int64(ttl.Seconds())
	}
	return []any{key, keyType, seconds, session.readKeyValue(ctx, key, keyType)}
}

// ReadPage reads the keyspace with a cursor, not an offset, so a later page is reached by
// scanning from the start and skipping the keys already shown. The cost grows with the
// offset, and the server offers no other way.
func (session *redisSession) ReadPage(
	ctx context.Context, read db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	target, isKeyRead := FindKeyRead(read)
	if !isKeyRead {
		return session.RunQuery(ctx, read.Text, window.Limit, read.Params)
	}

	startedAt := time.Now()
	wanted := window.Offset + window.Limit + 1
	keys, err := session.scanKeys(ctx, BuildMatchPattern(target.Prefix), wanted)
	if err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}

	named := make([]string, 0, len(keys))
	for _, key := range keys {
		if MatchesPrefix(key, target.Prefix) {
			named = append(named, key)
		}
	}
	slices.Sort(named)

	rows := [][]any{}
	for _, key := range named {
		row := session.readKeyRow(ctx, key)
		if KeepsRow(row, target.Filter) {
			rows = append(rows, row)
		}
		if len(rows) >= wanted {
			break
		}
	}

	page := [][]any{}
	if window.Offset < len(rows) {
		end := min(window.Offset+window.Limit, len(rows))
		page = rows[window.Offset:end]
	}

	return db.QueryResult{
		Columns: KeyResultColumns, Rows: page, Elapsed: time.Since(startedAt),
		Truncated: len(rows) > window.Offset+len(page), Command: "SCAN",
	}, nil
}

func (session *redisSession) CountRead(
	ctx context.Context, read db.ComposedRead,
) (int64, bool, error) {
	target, isKeyRead := FindKeyRead(read)
	if !isKeyRead {
		return 0, false, nil
	}
	keys, err := session.scanKeys(ctx, BuildMatchPattern(target.Prefix), catalogKeyLimit)
	if err != nil {
		return 0, false, db.WrapDatabaseError(err)
	}
	// SCAN of a prefix has no count; the cap is the same one ListTables uses.
	if len(keys) >= catalogKeyLimit {
		return 0, false, nil
	}
	counted := int64(0)
	for _, key := range keys {
		if MatchesPrefix(key, target.Prefix) {
			counted++
		}
	}
	return counted, true, nil
}

// RunQuery runs the commands of a buffer in order, and returns the result of the last one.
func (session *redisSession) RunQuery(
	ctx context.Context, buffer string, rowLimit int, _ []any,
) (db.QueryResult, error) {
	startedAt := time.Now()
	commands := []RedisCommand{}
	for _, line := range session.Support.Language.SplitStatements(buffer) {
		if command, read := readRedisCommandLine(line); read {
			commands = append(commands, command)
		}
	}
	if len(commands) == 0 {
		return BuildReplyResult(nil, time.Since(startedAt)), nil
	}

	var reply any
	for _, command := range commands {
		answered, err := session.send(ctx, command)
		if err != nil {
			return db.QueryResult{}, db.WrapDatabaseError(err)
		}
		reply = answered
	}

	answered := BuildReplyResult(reply, time.Since(startedAt))
	result := db.BuildCappedResult(db.CappedRead{
		Rows: answered.Rows, RowLimit: rowLimit, Columns: answered.Columns,
		Elapsed: time.Since(startedAt), Command: commands[len(commands)-1].Name,
	})
	return result, nil
}

// StreamQuery reads the whole of what the buffer answers, one batch at a time, for an
// export. A SCAN is walked key by key, so a browse of a prefix streams without holding
// every row at once. Every other command answers once, and that answer is the whole of
// it: a GET exports the one key it names, and never the keyspace around it.
func (session *redisSession) StreamQuery(
	ctx context.Context, buffer string, params []any, batchSize int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	command, read := readRedisCommandLine(buffer)
	if !read || command.Name != "SCAN" {
		return session.streamCommandReply(ctx, buffer, params, onBatch)
	}
	keys, err := session.scanKeys(ctx, readMatchPattern(command), everyKey)
	if err != nil {
		return 0, err
	}
	slices.Sort(keys)

	batcher := db.NewRowBatcher(batchSize, onBatch)
	for _, key := range keys {
		row := session.readKeyRow(ctx, key)
		if batchErr := batcher.AddRow(row, KeyResultColumns); batchErr != nil {
			return batcher.CountRows(), batchErr
		}
	}
	if batchErr := batcher.FlushRows(KeyResultColumns); batchErr != nil {
		return batcher.CountRows(), batchErr
	}
	return batcher.CountRows(), nil
}

// readMatchPattern returns the pattern of a SCAN, which the MATCH word names. A SCAN that
// names none walks the whole keyspace, which is what the command itself does.
func readMatchPattern(command RedisCommand) string {
	for at, argument := range command.Args {
		if strings.EqualFold(argument, "MATCH") && at+1 < len(command.Args) {
			return command.Args[at+1]
		}
	}
	return ""
}

// streamCommandReply exports what one buffer of commands answered. The reply is the whole
// result, so it is read once and handed over in batches.
func (session *redisSession) streamCommandReply(
	ctx context.Context, buffer string, params []any,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	// A row limit below zero caps nothing, because an export wants every row.
	answered, err := session.RunQuery(ctx, buffer, -1, params)
	if err != nil {
		return 0, err
	}
	if len(answered.Rows) == 0 {
		return 0, nil
	}
	if batchErr := onBatch(answered.Rows, answered.Columns); batchErr != nil {
		return 0, batchErr
	}
	return int64(len(answered.Rows)), nil
}

// ListTables returns the prefixes of the keyspace, which are the closest thing Redis has
// to a relation.
func (session *redisSession) ListTables(ctx context.Context) ([]db.TableRef, error) {
	keys, err := session.scanKeys(ctx, "", catalogKeyLimit)
	if err != nil {
		return nil, db.WrapDatabaseError(err)
	}

	counts := map[string]int64{}
	for _, key := range keys {
		counts[ReadKeyPrefix(key)]++
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	slices.Sort(names)

	tables := make([]db.TableRef, 0, len(names))
	for _, name := range names {
		tables = append(tables, db.TableRef{
			Schema: session.Descriptor.DefaultSchema, Name: name,
			Kind: db.RelationTable, EstimatedRows: counts[name],
		})
	}
	return tables, nil
}

// DescribeTable returns the same four columns for every prefix, because a key has no
// schema.
func (session *redisSession) DescribeTable(
	_ context.Context, table db.TableRef,
) (db.TableDetail, error) {
	return db.TableDetail{Table: table, Columns: KeyColumns}, nil
}

func (session *redisSession) ListIndexes(context.Context, db.TableRef) ([]db.IndexDetail, error) {
	return nil, nil
}

func (session *redisSession) ListConstraints(
	context.Context, db.TableRef,
) ([]db.ConstraintDetail, error) {
	return nil, nil
}

// BuildTableDDL reports what a prefix holds now, because nothing ever created it.
func (session *redisSession) BuildTableDDL(
	ctx context.Context, table db.TableRef,
) ([]string, error) {
	pattern := BuildMatchPattern(table.Name)
	keys, err := session.scanKeys(ctx, pattern, catalogKeyLimit)
	if err != nil {
		return nil, db.WrapDatabaseError(err)
	}

	named := make([]string, 0, len(keys))
	for _, key := range keys {
		if MatchesPrefix(key, table.Name) {
			named = append(named, key)
		}
	}

	types := map[string]int{}
	order := []string{}
	sampled := named
	if len(sampled) > ddlTypeSample {
		sampled = sampled[:ddlTypeSample]
	}
	for _, key := range sampled {
		keyType, typeErr := session.client.Type(ctx, key).Result()
		if typeErr != nil {
			continue
		}
		if _, held := types[keyType]; !held {
			order = append(order, keyType)
		}
		types[keyType]++
	}
	slices.Sort(order)

	lines := []string{
		fmt.Sprintf("# %d keys match %s", len(named), pattern),
		"# a prefix is a naming habit rather than an object, so there is nothing that made it",
	}
	for _, keyType := range order {
		lines = append(lines, fmt.Sprintf("#   %d of type %s", types[keyType], keyType))
	}
	return lines, nil
}

func (session *redisSession) BuildObjectDDL(
	context.Context, db.SchemaObject,
) ([]string, error) {
	return []string{"# redis keeps no object of this kind"}, nil
}

// CheckStatement returns nothing: only the server can check a command, and only by
// running it.
func (session *redisSession) CheckStatement(
	context.Context, string,
) (db.StatementProblem, bool) {
	return db.StatementProblem{}, false
}

// ExplainQuery is refused: a command names what it does, so the server plans nothing.
func (session *redisSession) ExplainQuery(
	context.Context, string, bool,
) (db.QueryPlan, error) {
	return db.QueryPlan{}, db.NewUnsupportedError("plan a statement")
}

// ApplyChanges runs the staged work of the grid as commands. Each row is named by its key.
// The commands go inside a MULTI, so the server runs the whole set with nothing of another
// connection in between, and a set that cannot be built is never sent at all.
func (session *redisSession) ApplyChanges(ctx context.Context, changes []db.Change) error {
	commands := make([]RedisCommand, 0, len(changes))
	for _, change := range changes {
		command, err := ReadRedisCommand(change)
		if err != nil {
			return err
		}
		commands = append(commands, command)
	}
	if len(commands) == 0 {
		return nil
	}

	_, err := session.client.TxPipelined(ctx, func(queue redis.Pipeliner) error {
		for _, command := range commands {
			args := make([]any, 0, len(command.Args)+1)
			args = append(args, command.Name)
			for _, argument := range command.Args {
				args = append(args, argument)
			}
			queue.Do(ctx, args...)
		}
		return nil
	})
	if err != nil {
		return db.WrapDatabaseError(err)
	}
	return nil
}

func (session *redisSession) ListActivity(ctx context.Context) ([]db.Activity, error) {
	listed, err := session.client.ClientList(ctx).Result()
	if err != nil {
		return nil, db.WrapDatabaseError(err)
	}
	activity := []db.Activity{}
	for line := range strings.SplitSeq(listed, "\n") {
		if entry, read := ReadClientLine(line); read {
			activity = append(activity, entry)
		}
	}
	return activity, nil
}

func (session *redisSession) CancelBackend(
	ctx context.Context, pid int64, _ bool,
) (bool, error) {
	answered, err := session.send(ctx, RedisCommand{
		Name: "CLIENT", Args: []string{"KILL", "ID", strconv.FormatInt(pid, 10)},
	})
	if err != nil {
		return false, db.WrapDatabaseError(err)
	}
	return db.ReadNonNegativeCount(answered) > 0, nil
}

// CancelRunningQuery is refused: one command at a time, and each one finishes, so none is
// ever running.
func (session *redisSession) CancelRunningQuery(context.Context) (bool, error) {
	return false, db.NewUnsupportedError("cancel a running statement")
}

func (session *redisSession) Ping(ctx context.Context) error {
	return session.client.Ping(ctx).Err()
}

func (session *redisSession) Close() error {
	return session.client.Close()
}

// redisAdapter opens a connection on a Redis server.
type redisAdapter struct{ support db.EngineSupport }

// NewAdapter returns the adapter that opens a Redis server.
func NewAdapter(support db.EngineSupport) db.Adapter {
	return &redisAdapter{support: support}
}

var databasePrefix = regexp.MustCompile(`(?i)^db`)

// ReadDatabaseIndex returns the database of a profile. Redis numbers its databases.
func ReadDatabaseIndex(profile cfg.Profile) (int, error) {
	written := databasePrefix.ReplaceAllString(strings.TrimSpace(profile.Database), "")
	if written == "" {
		return 0, nil
	}
	index, err := strconv.Atoi(written)
	if err != nil || index < 0 {
		return 0, db.NewDatabaseError("%q is not a database number", profile.Database)
	}
	return index, nil
}

var redisVersion = regexp.MustCompile(`redis_version:([^\r\n]+)`)

// ReadServerVersion reads the version out of the INFO block.
func ReadServerVersion(info string) string {
	found := redisVersion.FindStringSubmatch(info)
	if found == nil {
		return "unknown"
	}
	return strings.TrimSpace(found[1])
}

func (adapter *redisAdapter) Connect(
	ctx context.Context, profile cfg.Profile, password string,
) (db.Session, error) {
	index, err := ReadDatabaseIndex(profile)
	if err != nil {
		return nil, err
	}

	options := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", profile.Host, profile.Port),
		Username: profile.User,
		Password: password,
		DB:       index,
	}
	// A profile that names no mode connects in the clear, as a Redis client does.
	options.TLSConfig = db.BuildPolicyTLS(
		core.ResolveSSLPolicy(profile.SSLMode), profile.Host)

	client := redis.NewClient(options)
	info, infoErr := client.Info(ctx, "server").Result()
	if infoErr != nil {
		_ = client.Close()
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, infoErr), infoErr)
	}

	return &redisSession{
		SessionFacts: db.SessionFacts{
			Descriptor: db.SessionDescriptor{
				Profile: profile, ServerVersion: ReadServerVersion(info),
				DefaultSchema: fmt.Sprintf("db%d", index),
			},
			Support: adapter.support,
		},
		client: client,
	}, nil
}

// The compiler reports a part of the port this session has not answered for.
var _ db.Session = (*redisSession)(nil)
