package mongo

import (
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// MongoDB takes a call rather than SQL, and a call carries a document as its argument.
// A statement runs to the end of the line, or to a semicolon, and a document inside it
// spans as many lines as it needs.

// readMethods are the calls that only read, which the client colours and rates.
var readMethods = strings.Fields(`find findOne countDocuments count estimatedDocumentCount
	distinct aggregate getIndexes stats dataSize storageSize totalSize totalIndexSize
	getCollectionNames getCollectionInfos listCollections getName explain hello serverStatus
	currentOp sort limit skip projection hint collation batchSize allowDiskUse pretty toArray`)

// writeMethods are the calls that write, which are confirmed like a write.
var writeMethods = strings.Fields(`insertOne insertMany insert updateOne updateMany update
	replaceOne save findOneAndUpdate findOneAndReplace bulkWrite createIndex createIndexes
	createCollection createView renameCollection runCommand adminCommand killOp`)

// deleteMethods are the calls that remove data, confirmed like a delete.
var deleteMethods = strings.Fields(`deleteOne deleteMany remove findOneAndDelete dropIndex
	dropIndexes`)

// sweepingMethods are the calls that remove everything they are called on.
var sweepingMethods = strings.Fields(`drop dropDatabase`)

// wideMethods are the calls that reach every document their filter matches, so an empty
// filter reaches the whole collection.
var wideMethods = map[string]bool{"deleteMany": true, "updateMany": true, "remove": true}

// commandCallMethods are the calls that take a command document and hand it to the server
// as it is. The name of the call says nothing about what the command does, so the first
// key of the document decides the risk.
var commandCallMethods = map[string]bool{"runCommand": true, "adminCommand": true}

// readCommandNames are the commands that only read. A command in no list below counts as
// a write, as an unknown call does.
var readCommandNames = strings.Fields(`find getMore count distinct listCollections
	listIndexes listDatabases listCommands dbStats collStats connPoolStats serverStatus
	hostInfo buildInfo hello isMaster ping connectionStatus whatsmyuri currentOp top
	validate getParameter usersInfo rolesInfo replSetGetStatus getLog`)

// deleteCommandNames are the commands that remove data or an object, confirmed like a
// delete. findAndModify is here because it can remove the document it finds.
var deleteCommandNames = strings.Fields(`delete findAndModify dropIndexes dropIndex
	dropSearchIndex dropUser dropRole`)

// sweepingCommandNames are the commands that remove everything they are called on.
var sweepingCommandNames = strings.Fields(`dropDatabase drop emptycapped shutdown
	dropAllUsersFromDatabase dropAllRolesFromDatabase killAllSessions
	killAllSessionsByPattern`)

var riskByCommandName = map[string]statement.WriteRisk{}

// catalogMethods are the calls that leave the catalog of this client stale.
var catalogMethods = map[string]bool{
	"drop": true, "dropDatabase": true, "createCollection": true, "createView": true,
	"renameCollection": true, "createIndex": true, "createIndexes": true,
	"dropIndex": true, "dropIndexes": true, "runCommand": true, "adminCommand": true,
}

// explainMethods are the calls the server plans.
var explainMethods = map[string]bool{
	"find": true, "aggregate": true, "count": true, "countDocuments": true, "distinct": true,
}

// methodOrder holds every call in the order they are named above, which is the order the
// completion offers them.
var methodOrder = []string{}

var knownMethods = map[string]bool{}
var riskByMethod = map[string]statement.WriteRisk{}

func init() {
	for _, group := range []struct {
		names []string
		risk  statement.WriteRisk
	}{
		{readMethods, statement.RiskNone},
		{writeMethods, statement.RiskWrite},
		{deleteMethods, statement.RiskDelete},
		{sweepingMethods, statement.RiskEveryRow},
	} {
		for _, name := range group.names {
			methodOrder = append(methodOrder, name)
			knownMethods[name] = true
			riskByMethod[name] = group.risk
		}
	}
	for _, name := range []string{siblingCall, collectionCall} {
		methodOrder = append(methodOrder, name)
		knownMethods[name] = true
		riskByMethod[name] = statement.RiskNone
	}
	for _, group := range []struct {
		names []string
		risk  statement.WriteRisk
	}{
		{readCommandNames, statement.RiskNone},
		{deleteCommandNames, statement.RiskDelete},
		{sweepingCommandNames, statement.RiskEveryRow},
	} {
		for _, name := range group.names {
			riskByCommandName[strings.ToLower(name)] = group.risk
		}
	}
}

// mongoLanguage reads a buffer of MongoDB calls.
type mongoLanguage struct{}

// Language reads a buffer of MongoDB calls, one statement to a line.
var Language language.Language = mongoLanguage{}

// SplitStatementRanges returns one range per statement. A statement ends at a semicolon
// or at the end of a line, and a document that is still open carries it on. A line that
// opens with a dot carries the call chain of the line before it on too.
func (mongoLanguage) SplitStatementRanges(text string) []statement.StatementRange {
	ranges := []statement.StatementRange{}
	depth := 0
	start := 0
	index := 0
	lastMeaning := byte(0)

	push := func(end int) {
		slice := text[start:end]
		leading := len(slice) - len(strings.TrimLeft(slice, " \t\r\n\v\f"))
		trimmed := strings.TrimSpace(slice)
		if trimmed != "" {
			ranges = append(ranges, statement.StatementRange{
				Text: trimmed, Start: start + leading, End: start + leading + len(trimmed),
			})
		}
	}

	for index < len(text) {
		if past, skipped := skipText(text, index, lastMeaning); skipped {
			lastMeaning = text[index]
			index = past
			continue
		}
		character := text[index]
		switch character {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ';':
			if depth == 0 {
				push(index)
				start = index + 1
			}
		case '\n':
			if depth == 0 && !carriesOn(text, index+1) {
				push(index)
				start = index + 1
			}
		}
		if !isBlank(character) {
			lastMeaning = character
		}
		index++
	}
	push(len(text))
	return ranges
}

// carriesOn is true where the next line joins the statement before it, which is a line
// that opens with a dot.
func carriesOn(text string, at int) bool {
	for at < len(text) {
		if isBlank(text[at]) {
			at++
			continue
		}
		past, skipped := skipComment(text, at)
		if !skipped {
			return text[at] == '.'
		}
		at = past
	}
	return false
}

func (held mongoLanguage) SplitStatements(text string) []string {
	ranges := held.SplitStatementRanges(text)
	statements := make([]string, 0, len(ranges))
	for _, one := range ranges {
		statements = append(statements, one.Text)
	}
	return statements
}

func (held mongoLanguage) ReadStatementAtOffset(text string, offset int) string {
	ranges := held.SplitStatementRanges(text)
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

// Tokenize returns the tokens of the buffer, which the editor colours.
func (mongoLanguage) Tokenize(text string) []syntax.Token {
	tokens := []syntax.Token{}
	index := 0
	lastMeaning := byte(0)

	for index < len(text) {
		character := text[index]
		switch {
		case isBlank(character):
			index++
			continue
		case character == '/' || character == '"' || character == '\'' || character == '`':
			past, kind, unterminated := readTextToken(text, index, lastMeaning)
			if past > index {
				tokens = append(tokens, syntax.Token{
					Kind: kind, Start: index, End: past, Unterminated: unterminated,
				})
				lastMeaning = character
				index = past
				continue
			}
		case isDigit(character) || (character == '-' && index+1 < len(text) && isDigit(text[index+1])):
			past := index + 1
			for past < len(text) && (isDigit(text[past]) || text[past] == '.' ||
				text[past] == 'e' || text[past] == 'E' ||
				((text[past] == '+' || text[past] == '-') &&
					(text[past-1] == 'e' || text[past-1] == 'E'))) {
				past++
			}
			tokens = append(tokens, syntax.Token{Kind: syntax.TokenNumber, Start: index, End: past})
			lastMeaning = character
			index = past
			continue
		case isNameStart(character):
			past := index
			for past < len(text) && isNamePart(text[past]) {
				past++
			}
			tokens = append(tokens, syntax.Token{
				Kind:  readNameKind(text, index, past),
				Start: index, End: past,
			})
			lastMeaning = character
			index = past
			continue
		}
		tokens = append(tokens, syntax.Token{
			Kind: syntax.TokenOperator, Start: index, End: index + 1,
		})
		lastMeaning = character
		index++
	}
	return tokens
}

// readTextToken returns where a comment, a quoted value or a regular expression ends,
// and the kind it is coloured as.
func readTextToken(text string, at int, lastMeaning byte) (int, syntax.TokenKind, bool) {
	if past, skipped := skipComment(text, at); skipped {
		return past, syntax.TokenComment, false
	}
	if text[at] == '/' {
		if !strings.ContainsRune(":,[({=", rune(lastMeaning)) {
			return at, syntax.TokenOperator, false
		}
		return skipRegex(text, at), syntax.TokenString, false
	}
	past := skipQuoted(text, at)
	return past, syntax.TokenString, past >= len(text) && text[len(text)-1] != text[at]
}

// readNameKind returns how a bare name is coloured: the word every statement opens
// with, a call, an operator of a document, or a name.
func readNameKind(text string, start, end int) syntax.TokenKind {
	word := text[start:end]
	if word == databaseWord && !followsDot(text, start) {
		return syntax.TokenKeyword
	}
	if strings.HasPrefix(word, "$") {
		return syntax.TokenType
	}
	index := end
	for index < len(text) && isBlank(text[index]) {
		index++
	}
	if index < len(text) && text[index] == '(' && knownMethods[word] {
		return syntax.TokenKeyword
	}
	return syntax.TokenIdentifier
}

// followsDot is true where the name stands after a dot, which makes it a call or a
// collection rather than the word a statement opens with.
func followsDot(text string, start int) bool {
	for at := start - 1; at >= 0; at-- {
		if isBlank(text[at]) {
			continue
		}
		return text[at] == '.'
	}
	return false
}

// FormatStatement writes each statement on its own line, with the blanks outside its
// text collapsed. The document of a call is left as the user wrote it, because the
// helpers of the shell have no shorter form.
func (held mongoLanguage) FormatStatement(text string) string {
	lines := []string{}
	for _, one := range held.SplitStatementRanges(text) {
		lines = append(lines, collapseOutsideText(one.Text))
	}
	return strings.Join(lines, "\n")
}

// collapseOutsideText turns every run of blanks that is not inside a quoted value into
// one space, and drops the space that stands beside a bracket or a comma.
func collapseOutsideText(text string) string {
	var built strings.Builder
	index := 0
	lastMeaning := byte(0)
	pending := false

	for index < len(text) {
		if past, skipped := skipText(text, index, lastMeaning); skipped {
			if pending && built.Len() > 0 {
				built.WriteByte(' ')
			}
			pending = false
			built.WriteString(text[index:past])
			lastMeaning = text[index]
			index = past
			continue
		}
		character := text[index]
		if isBlank(character) {
			pending = true
			index++
			continue
		}
		if pending && built.Len() > 0 && !strings.ContainsRune(",)]}", rune(character)) &&
			!strings.ContainsRune("([{", rune(lastMeaning)) {
			built.WriteByte(' ')
		}
		pending = false
		built.WriteByte(character)
		lastMeaning = character
		index++
	}
	return built.String()
}

// ResolveWriteRisk weighs the buffer. A call that is in no list counts as a write, and a
// call that reaches every document its filter matches counts as the whole collection
// where the filter is empty.
func (held mongoLanguage) ResolveWriteRisk(text string) statement.WriteRisk {
	statements := held.SplitStatements(text)
	risks := make([]statement.WriteRisk, 0, len(statements))
	for _, written := range statements {
		risks = append(risks, resolveStatementRisk(written))
	}
	return statement.ResolveStrongestRisk(risks)
}

func resolveStatementRisk(written string) statement.WriteRisk {
	parsed, _, ok := ParseStatement(written)
	if !ok {
		return statement.RiskWrite
	}
	method := parsed.ReadMethod()
	risk, known := riskByMethod[method]
	if !known {
		return statement.RiskWrite
	}
	if commandCallMethods[method] {
		return resolveCommandRisk(parsed.Calls[0].ReadArgument(0))
	}
	if method == "aggregate" {
		if stageRisk, writes := resolvePipelineRisk(parsed.Calls[0].ReadArgument(0)); writes {
			return stageRisk
		}
	}
	if wideMethods[method] && isEmptyFilter(parsed.Calls[0].ReadArgument(0)) &&
		!readsJustOne(parsed.Calls[0]) {
		return statement.RiskEveryRow
	}
	return risk
}

// writingStageRisks names the pipeline stages that write, and what each one costs. `$out`
// replaces every document of the collection it names, and `$merge` writes into it.
var writingStageRisks = map[string]statement.WriteRisk{
	"$out":   statement.RiskEveryRow,
	"$merge": statement.RiskWrite,
}

// resolvePipelineRisk weighs the stages of a pipeline. An aggregation reads until a stage
// writes, so the stages decide the risk, not the name of the call. A pipeline this client
// cannot read counts as a write, because a stage it cannot see may be one that writes.
func resolvePipelineRisk(written string) (statement.WriteRisk, bool) {
	pipeline, err := ReadArray(written)
	if err != nil {
		return statement.RiskWrite, true
	}
	return resolvePipelineStageRisk(pipeline)
}

// resolvePipelineStageRisk weighs the stages of a pipeline that is already read.
func resolvePipelineStageRisk(pipeline bson.A) (statement.WriteRisk, bool) {
	found := []statement.WriteRisk{}
	for _, stage := range pipeline {
		document, isDocument := stage.(bson.D)
		if !isDocument {
			return statement.RiskWrite, true
		}
		for _, element := range document {
			if risk, writes := writingStageRisks[element.Key]; writes {
				found = append(found, risk)
			}
		}
	}
	if len(found) == 0 {
		return statement.RiskNone, false
	}
	return statement.ResolveStrongestRisk(found), true
}

// resolveCommandRisk weighs the document a runCommand or an adminCommand carries. The
// first key of a command document is the name of the command, so that name decides. A
// document this client cannot read counts as a write, and so does a command it does not
// know: the server takes any command on this path, a dropDatabase among them.
func resolveCommandRisk(written string) statement.WriteRisk {
	document, err := ReadDocument(written)
	if err != nil || len(document) == 0 {
		return statement.RiskWrite
	}
	name := strings.ToLower(document[0].Key)
	if name == "aggregate" {
		risk, writes := resolveCommandPipelineRisk(document)
		if writes {
			return risk
		}
		return statement.RiskNone
	}
	if risk, known := riskByCommandName[name]; known {
		return risk
	}
	return statement.RiskWrite
}

// resolveCommandPipelineRisk weighs the pipeline of an aggregate command, which is a
// stage list like the one the call of a collection takes.
func resolveCommandPipelineRisk(document bson.D) (statement.WriteRisk, bool) {
	for _, element := range document {
		if element.Key != "pipeline" {
			continue
		}
		pipeline, isArray := element.Value.(bson.A)
		if !isArray {
			return statement.RiskWrite, true
		}
		return resolvePipelineStageRisk(pipeline)
	}
	return statement.RiskWrite, true
}

// isEmptyFilter is true for a filter that names nothing, which matches every document.
func isEmptyFilter(written string) bool {
	trimmed := strings.TrimSpace(written)
	return trimmed == "" || trimmed == "{}"
}

// HoldsRowLimit is true for a statement that bounds its own result: a `limit` call, or a
// `findOne`, which returns one document by its name.
func (mongoLanguage) HoldsRowLimit(text string) bool {
	parsed, _, ok := ParseStatement(text)
	if !ok {
		return false
	}
	if parsed.ReadMethod() == "findOne" {
		return true
	}
	_, held := parsed.FindCall("limit")
	return held
}

// ChangesCatalog is true where the call adds, removes or renames something the tree
// draws.
func (mongoLanguage) ChangesCatalog(text string) bool {
	parsed, _, ok := ParseStatement(text)
	return ok && catalogMethods[parsed.ReadMethod()]
}

// CanExplain is true where the server plans the call.
func (mongoLanguage) CanExplain(text string) bool {
	parsed, _, ok := ParseStatement(text)
	return ok && explainMethods[parsed.ReadMethod()]
}

// FindLocalDiagnostics returns the faults the client can find without the server: a
// statement it cannot read, a call it does not know, and an argument that is no value.
func (held mongoLanguage) FindLocalDiagnostics(
	text string, _ editor.SchemaKnowledge,
) []editor.Diagnostic {
	problems := []editor.Diagnostic{}
	for _, one := range held.SplitStatementRanges(text) {
		parsed, fault, ok := ParseStatement(one.Text)
		if !ok {
			problems = append(problems, editor.Diagnostic{
				Message: fault.Message,
				Start:   one.Start + fault.Start, End: one.Start + fault.End,
			})
			continue
		}
		problems = append(problems, findCallProblems(parsed, one.Start)...)
	}
	return problems
}

// findCallProblems returns the faults of the calls of one statement.
func findCallProblems(parsed Statement, offset int) []editor.Diagnostic {
	problems := []editor.Diagnostic{}
	for _, call := range parsed.Calls {
		if !knownMethods[call.Name] {
			problems = append(problems, editor.Diagnostic{
				Message: call.Name + " is not a call this client knows",
				Start:   offset + call.Start, End: offset + call.End,
			})
			continue
		}
		for _, argument := range call.Args {
			if _, err := ReadValue(argument); err != nil {
				problems = append(problems, editor.Diagnostic{
					Message: "the argument of " + call.Name + " is no value: " + err.Error(),
					Start:   offset + call.Start, End: offset + call.End,
				})
				break
			}
		}
	}
	return problems
}

// completionLimit is how many suggestions the list holds.
const completionLimit = 50

// LineComment returns the two slashes the shell comments a line out with.
func (mongoLanguage) LineComment() string {
	return "//"
}

// BuildCompletions completes the word every statement opens with, the collections after
// it, and the calls after those.
func (mongoLanguage) BuildCompletions(
	prefix string, sources editor.CompletionSources, _ editor.CompletionContext,
) []editor.Completion {
	if prefix == "" {
		return nil
	}
	head, tail := splitPrefix(prefix)

	if head == "" {
		if strings.HasPrefix(databaseWord, strings.ToLower(tail)) {
			return []editor.Completion{{Text: databaseWord, Kind: editor.CompleteKeyword}}
		}
		return nil
	}
	if head == databaseWord {
		return offerCollections(head, tail, sources)
	}
	return offerMethods(head, tail)
}

// splitPrefix returns the part of the prefix that is settled and the word being typed.
func splitPrefix(prefix string) (string, string) {
	at := strings.LastIndexByte(prefix, '.')
	if at == -1 {
		return "", prefix
	}
	return prefix[:at], prefix[at+1:]
}

// offerCollections offers the collections of the catalog and the calls made on the
// database itself.
func offerCollections(
	head, tail string, sources editor.CompletionSources,
) []editor.Completion {
	lowered := strings.ToLower(tail)
	offered := []editor.Completion{}
	seen := map[string]bool{}

	names := make([]string, 0, len(sources.Tables))
	for _, name := range sources.Tables {
		// The catalog offers a name both bare and with its database, and a call chain
		// takes the bare one only.
		if strings.Contains(name, ".") || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if strings.HasPrefix(strings.ToLower(name), lowered) {
			offered = append(offered, editor.Completion{
				Text: head + "." + name, Kind: editor.CompleteTable,
			})
		}
	}
	for _, name := range []string{siblingCall, collectionCall, "runCommand",
		"getCollectionNames", "adminCommand", "createCollection", "dropDatabase"} {
		if strings.HasPrefix(strings.ToLower(name), lowered) {
			offered = append(offered, editor.Completion{
				Text: head + "." + name, Kind: editor.CompleteFunction,
			})
		}
	}
	return capCompletions(offered)
}

// offerMethods offers the calls made on a collection.
func offerMethods(head, tail string) []editor.Completion {
	lowered := strings.ToLower(tail)
	offered := []editor.Completion{}
	for _, name := range methodOrder {
		if strings.HasPrefix(strings.ToLower(name), lowered) {
			offered = append(offered, editor.Completion{
				Text: head + "." + name, Kind: editor.CompleteFunction,
			})
		}
	}
	return capCompletions(offered)
}

func capCompletions(offered []editor.Completion) []editor.Completion {
	if len(offered) > completionLimit {
		return offered[:completionLimit]
	}
	return offered
}
