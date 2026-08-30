package redis

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Redis reads one command per line, not a statement ended by a semicolon. A key can
// hold any character, a semicolon too, so only a line break ends a command.

// redisReadCommands are the read-only commands the client colours, completes and rates.
var redisReadCommands = strings.Fields(`GET MGET STRLEN EXISTS TYPE TTL PTTL KEYS SCAN RANDOMKEY
	DBSIZE HGET HGETALL HKEYS HVALS HLEN HEXISTS HSCAN LRANGE LLEN LINDEX SMEMBERS SCARD
	SISMEMBER SSCAN ZRANGE ZRANGEBYSCORE ZCARD ZSCORE ZSCAN XRANGE XLEN OBJECT MEMORY INFO
	CONFIG CLIENT COMMAND PING ECHO TIME LASTSAVE SLOWLOG EVAL_RO EVALSHA_RO FCALL_RO`)

// redisWriteCommands are the commands that write keys, which are confirmed like a write.
var redisWriteCommands = strings.Fields(`SET SETEX SETNX MSET GETSET APPEND INCR INCRBY DECR
	DECRBY HSET HSETNX HINCRBY LPUSH RPUSH LSET LINSERT SADD ZADD ZINCRBY XADD EXPIRE PEXPIRE
	EXPIREAT PERSIST RENAME RENAMENX COPY RESTORE SETRANGE`)

// redisDeleteCommands are the commands that remove data, confirmed like a delete.
var redisDeleteCommands = strings.Fields(`DEL UNLINK HDEL LPOP RPOP LREM LTRIM SREM SPOP ZREM
	ZREMRANGEBYSCORE ZREMRANGEBYRANK XDEL XTRIM GETDEL`)

// redisSweepingCommands are the commands that remove everything, and name no key.
var redisSweepingCommands = strings.Fields(
	`FLUSHDB FLUSHALL SWAPDB SCRIPT FUNCTION SHUTDOWN`)

// redisScriptCommands run a body on the server, and that body can call any command, a
// FLUSHALL among them. The words of the call say nothing about what the body does, so a
// script is rated at the highest risk. The `_RO` names the server itself holds to reads
// are in the read list above.
var redisScriptCommands = strings.Fields(`EVAL EVALSHA FCALL`)

// redisContainerReads names, for a command that holds subcommands, the subcommands that
// only read. One name holds both a read and a write: CONFIG GET reads and CONFIG SET
// writes, so the name alone cannot rate the command. A subcommand that is not named here
// counts as a write.
var redisContainerReads = map[string][]string{
	"CONFIG":  {"GET", "HELP"},
	"CLIENT":  {"ID", "INFO", "LIST", "GETNAME", "GETREDIR", "TRACKINGINFO", "HELP"},
	"MEMORY":  {"USAGE", "STATS", "DOCTOR", "MALLOC-STATS", "HELP"},
	"SLOWLOG": {"GET", "LEN", "HELP"},
	"OBJECT":  {"ENCODING", "FREQ", "IDLETIME", "REFCOUNT", "HELP"},
	"COMMAND": {"COUNT", "DOCS", "GETKEYS", "GETKEYSANDFLAGS", "INFO", "LIST", "HELP"},
}

// redisCommandOrder holds every command in the order they are named above, which is
// the order the completion offers them.
var redisCommandOrder = []string{}

var redisKnownCommands = map[string]bool{}
var redisRiskByCommand = map[string]statement.WriteRisk{}

func init() {
	for _, name := range redisReadCommands {
		redisCommandOrder = append(redisCommandOrder, name)
		redisKnownCommands[name] = true
		redisRiskByCommand[name] = statement.RiskNone
	}
	for _, name := range redisWriteCommands {
		redisCommandOrder = append(redisCommandOrder, name)
		redisKnownCommands[name] = true
		redisRiskByCommand[name] = statement.RiskWrite
	}
	for _, name := range redisDeleteCommands {
		redisCommandOrder = append(redisCommandOrder, name)
		redisKnownCommands[name] = true
		redisRiskByCommand[name] = statement.RiskDelete
	}
	for _, name := range redisSweepingCommands {
		redisCommandOrder = append(redisCommandOrder, name)
		redisKnownCommands[name] = true
		redisRiskByCommand[name] = statement.RiskEveryRow
	}
	for _, name := range redisScriptCommands {
		redisCommandOrder = append(redisCommandOrder, name)
		redisKnownCommands[name] = true
		redisRiskByCommand[name] = statement.RiskEveryRow
	}
	for name := range strings.FieldsSeq(
		"MULTI EXEC DISCARD WATCH UNWATCH SELECT AUTH SUBSCRIBE PUBLISH") {
		redisCommandOrder = append(redisCommandOrder, name)
		redisKnownCommands[name] = true
	}
}

// RedisWord is one word of a command. Quotes keep a word together over its spaces.
type RedisWord struct {
	Text         string
	Start        int
	End          int
	Quoted       bool
	Unterminated bool
}

// scanQuotedRedisWord reads a word inside quote marks. A backslash escapes the next
// character, so a value with a space or a quote is still one argument.
func scanQuotedRedisWord(line string, start, offset int) (RedisWord, int) {
	quote := line[start]
	index := start + 1
	var text strings.Builder
	closed := false

	for index < len(line) {
		character := line[index]
		if character == '\\' && index+1 < len(line) {
			text.WriteByte(line[index+1])
			index += 2
			continue
		}
		if character == quote {
			index++
			closed = true
			break
		}
		text.WriteByte(character)
		index++
	}

	return RedisWord{
		Text: text.String(), Start: offset + start, End: offset + index,
		Quoted: true, Unterminated: !closed,
	}, index
}

// scanPlainRedisWord reads a word that runs to the next space.
func scanPlainRedisWord(line string, start, offset int) (RedisWord, int) {
	index := start
	for index < len(line) && !unicode.IsSpace(rune(line[index])) {
		index++
	}
	return RedisWord{
		Text: line[start:index], Start: offset + start, End: offset + index,
	}, index
}

// ReadRedisWords returns the words of one line. Redis accepts both quote marks.
func ReadRedisWords(line string, offset int) []RedisWord {
	words := []RedisWord{}
	index := 0

	for index < len(line) {
		for index < len(line) && unicode.IsSpace(rune(line[index])) {
			index++
		}
		if index >= len(line) {
			break
		}
		quote := line[index]
		var word RedisWord
		if quote == '"' || quote == '\'' {
			word, index = scanQuotedRedisWord(line, index, offset)
		} else {
			word, index = scanPlainRedisWord(line, index, offset)
		}
		words = append(words, word)
	}
	return words
}

// readRedisCommandName returns the command name of a line, in capitals.
func readRedisCommandName(line string) string {
	words := ReadRedisWords(line, 0)
	if len(words) == 0 {
		return ""
	}
	return strings.ToUpper(words[0].Text)
}

var redisNumber = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// redisLanguage reads a buffer of Redis commands.
type redisLanguage struct{}

func (redisLanguage) Tokenize(text string) []syntax.Token {
	tokens := []syntax.Token{}
	lineStart := 0
	for line := range strings.SplitSeq(text, "\n") {
		for index, word := range ReadRedisWords(line, lineStart) {
			kind := syntax.TokenIdentifier
			switch {
			case index == 0:
				kind = syntax.TokenKeyword
			case word.Quoted:
				kind = syntax.TokenString
			case redisNumber.MatchString(word.Text):
				kind = syntax.TokenNumber
			}
			tokens = append(tokens, syntax.Token{
				Kind: kind, Start: word.Start, End: word.End, Unterminated: word.Unterminated,
			})
		}
		lineStart += len(line) + 1
	}
	return tokens
}

// SplitStatementRanges returns one range per line that is not empty, because a line
// break ends a command.
func (redisLanguage) SplitStatementRanges(text string) []statement.StatementRange {
	ranges := []statement.StatementRange{}
	start := 0
	for line := range strings.SplitSeq(text, "\n") {
		leading := len(line) - len(strings.TrimLeft(line, " \t\r"))
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			ranges = append(ranges, statement.StatementRange{
				Text: trimmed, Start: start + leading, End: start + leading + len(trimmed),
			})
		}
		start += len(line) + 1
	}
	return ranges
}

func (language redisLanguage) SplitStatements(text string) []string {
	ranges := language.SplitStatementRanges(text)
	statements := make([]string, 0, len(ranges))
	for _, one := range ranges {
		statements = append(statements, one.Text)
	}
	return statements
}

func (language redisLanguage) ReadStatementAtOffset(text string, offset int) string {
	ranges := language.SplitStatementRanges(text)
	for _, one := range ranges {
		if offset >= one.Start && offset <= one.End {
			return one.Text
		}
	}
	if len(ranges) > 0 {
		return ranges[len(ranges)-1].Text
	}
	return strings.TrimSpace(text)
}

// FormatStatement writes each command in capitals, with one space between its words.
func (language redisLanguage) FormatStatement(text string) string {
	lines := []string{}
	for _, one := range language.SplitStatementRanges(text) {
		words := ReadRedisWords(one.Text, 0)
		written := make([]string, 0, len(words))
		for index, word := range words {
			text := word.Text
			if word.Quoted {
				text = `"` + strings.ReplaceAll(word.Text, `"`, `\"`) + `"`
			}
			if index == 0 {
				text = strings.ToUpper(text)
			}
			written = append(written, text)
		}
		lines = append(lines, strings.Join(written, " "))
	}
	return strings.Join(lines, "\n")
}

// ResolveWriteRisk weighs the buffer. A command that is in no list counts as a write.
func (language redisLanguage) ResolveWriteRisk(text string) statement.WriteRisk {
	statements := language.SplitStatements(text)
	risks := make([]statement.WriteRisk, 0, len(statements))
	for _, line := range statements {
		risks = append(risks, resolveRedisCommandRisk(line))
	}
	return statement.ResolveStrongestRisk(risks)
}

// resolveRedisCommandRisk weighs one command. A command that holds subcommands is rated
// by its subcommand, because CONFIG SET writes where CONFIG GET reads.
func resolveRedisCommandRisk(line string) statement.WriteRisk {
	words := ReadRedisWords(line, 0)
	name := readRedisCommandName(line)
	if reads, holdsSubcommands := redisContainerReads[name]; holdsSubcommands && len(words) > 1 {
		if slices.Contains(reads, strings.ToUpper(words[1].Text)) {
			return statement.RiskNone
		}
		return statement.RiskWrite
	}
	risk, known := redisRiskByCommand[name]
	if !known {
		return statement.RiskWrite
	}
	return risk
}

// FindLocalDiagnostics returns the faults the client can find without the server: an
// unclosed quote and an unknown command. Only the server knows the arguments of each
// command.
func (redisLanguage) FindLocalDiagnostics(text string, _ editor.SchemaKnowledge) []editor.Diagnostic {
	problems := []editor.Diagnostic{}
	lineStart := 0

	for line := range strings.SplitSeq(text, "\n") {
		words := ReadRedisWords(line, lineStart)
		for _, word := range words {
			if word.Unterminated {
				problems = append(problems, editor.Diagnostic{
					Message: "this quote never closes", Start: word.Start, End: word.End,
				})
			}
		}
		if len(words) > 0 && !words[0].Quoted &&
			!redisKnownCommands[strings.ToUpper(words[0].Text)] {
			problems = append(problems, editor.Diagnostic{
				Message: strings.ToUpper(words[0].Text) + " is not a command this client knows",
				Start:   words[0].Start, End: words[0].End,
			})
		}
		lineStart += len(line) + 1
	}
	return problems
}

// ChangesCatalog is always false: the tree is built by a scan of the key space, not
// from a catalog, so a write changes only what the next scan finds.
func (redisLanguage) ChangesCatalog(string) bool { return false }

// CanExplain is always false: a command says what it does, so the server plans nothing.
func (redisLanguage) CanExplain(string) bool { return false }

// BuildCompletions completes a command in the first word, and a key after it.
// LineComment returns nothing, because the protocol of the key store has no comment.
func (redisLanguage) LineComment() string {
	return ""
}

func (redisLanguage) BuildCompletions(
	prefix string, sources editor.CompletionSources, _ editor.CompletionContext,
) []editor.Completion {
	if prefix == "" {
		return nil
	}
	wanted := strings.ToUpper(prefix)

	completions := []editor.Completion{}
	for _, name := range redisCommandOrder {
		if strings.HasPrefix(name, wanted) {
			completions = append(completions, editor.Completion{Text: name, Kind: editor.CompleteKeyword})
		}
	}
	lowered := strings.ToLower(prefix)
	for _, name := range sources.Tables {
		if strings.HasPrefix(strings.ToLower(name), lowered) {
			completions = append(completions, editor.Completion{Text: name, Kind: editor.CompleteTable})
		}
	}
	if len(completions) > 50 {
		return completions[:50]
	}
	return completions
}

// Language reads a buffer of Redis commands, one to a line.
var Language language.Language = redisLanguage{}
