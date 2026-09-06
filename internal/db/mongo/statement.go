package mongo

import (
	"errors"
	"strings"
)

// A statement is a shell call chain with a database, collection, and method calls.

// databaseWord is the word every statement starts with.
const databaseWord = "db"

// siblingCall is the method for selecting another database.
const siblingCall = "getSiblingDB"

// collectionCall is the method for selecting a collection by a quoted name.
const collectionCall = "getCollection"

// MethodCall is one call of a statement: its name and the text of each argument.
type MethodCall struct {
	Name string
	Args []string
	// Start and End are the method name offsets.
	Start int
	End   int
}

// ReadArgument returns an argument, or an empty string for an invalid index.
func (call MethodCall) ReadArgument(index int) string {
	if index < 0 || index >= len(call.Args) {
		return ""
	}
	return call.Args[index]
}

// Statement is one command of a buffer, read into the parts a session runs.
type Statement struct {
	// Empty for the connection database.
	Database string
	// Empty for a call on the database itself, such as runCommand.
	Collection string
	Calls      []MethodCall
	Text       string
}

// ReadMethod returns the first method name.
func (parsed Statement) ReadMethod() string {
	if len(parsed.Calls) == 0 {
		return ""
	}
	return parsed.Calls[0].Name
}

// FindCall returns the chained call of that name, and whether the statement has one.
func (parsed Statement) FindCall(name string) (MethodCall, bool) {
	for at, call := range parsed.Calls {
		if at > 0 && call.Name == name {
			return call, true
		}
	}
	return MethodCall{}, false
}

// SyntaxFault is a local syntax error and its offsets.
type SyntaxFault struct {
	Message string
	Start   int
	End     int
}

// newSyntaxError builds the error of a statement this client cannot read.
func newSyntaxError(message string) error {
	return errors.New(message)
}

// parser walks one statement.
type parser struct {
	text string
	at   int
}

// ParseStatement reads one statement into its parts, or returns the fault that stopped
// it.
func ParseStatement(text string) (Statement, SyntaxFault, bool) {
	read := &parser{text: text}
	parsed, fault, ok := read.parse()
	parsed.Text = strings.TrimSpace(text)
	return parsed, fault, ok
}

// ReadStatement reads one statement and returns its fault as an error.
func ReadStatement(text string) (Statement, error) {
	parsed, fault, ok := ParseStatement(text)
	if !ok {
		return Statement{}, newSyntaxError(fault.Message)
	}
	return parsed, nil
}

func (read *parser) parse() (Statement, SyntaxFault, bool) {
	parsed := Statement{}

	read.skipBlank()
	start := read.at
	if read.readName() != databaseWord {
		return parsed, SyntaxFault{
			Message: "a statement starts with db", Start: start, End: read.markEnd(start),
		}, false
	}

	for {
		read.skipBlank()
		if read.at >= len(read.text) || read.text[read.at] == ';' {
			break
		}
		if read.text[read.at] != '.' {
			return parsed, SyntaxFault{
				Message: "expected a dot before the call", Start: read.at, End: read.at + 1,
			}, false
		}
		read.at++

		read.skipBlank()
		nameStart := read.at
		name := read.readName()
		if name == "" {
			return parsed, SyntaxFault{
				Message: "expected a name after the dot", Start: nameStart,
				End: read.markEnd(nameStart),
			}, false
		}

		read.skipBlank()
		if read.at >= len(read.text) || read.text[read.at] != '(' {
			if fault, ok := read.takeName(&parsed, name, nameStart); !ok {
				return parsed, fault, false
			}
			continue
		}

		args, closed, fault := readCallArguments(read.text, read.at)
		if fault != nil {
			return parsed, *fault, false
		}
		read.at = closed
		if held, ok := read.takeCall(&parsed, MethodCall{
			Name: name, Args: args, Start: nameStart, End: nameStart + len(name),
		}); !ok {
			return parsed, held, false
		}
	}

	if len(parsed.Calls) == 0 {
		return parsed, SyntaxFault{
			Message: "the statement has no method call", Start: 0, End: len(read.text),
		}, false
	}
	return parsed, SyntaxFault{}, true
}

// takeName reads a collection name without parentheses.
func (read *parser) takeName(
	parsed *Statement, name string, start int,
) (SyntaxFault, bool) {
	if parsed.Collection != "" || len(parsed.Calls) > 0 {
		return SyntaxFault{
			Message: "missing parentheses after " + name,
			Start:   start, End: start + len(name),
		}, false
	}
	parsed.Collection = name
	return SyntaxFault{}, true
}

// takeCall reads a database selector, collection selector, or method call.
func (read *parser) takeCall(parsed *Statement, call MethodCall) (SyntaxFault, bool) {
	first := parsed.Collection == "" && len(parsed.Calls) == 0
	if first && (call.Name == siblingCall || call.Name == collectionCall) {
		name, err := ReadText(call.ReadArgument(0))
		if err != nil || name == "" {
			return SyntaxFault{
				Message: call.Name + " requires a non-empty database or collection name in quotes",
				Start:   call.Start, End: call.End,
			}, false
		}
		if call.Name == siblingCall {
			parsed.Database = name
			return SyntaxFault{}, true
		}
		parsed.Collection = name
		return SyntaxFault{}, true
	}

	parsed.Calls = append(parsed.Calls, call)
	return SyntaxFault{}, true
}

// ReadText reads an argument that names something, which is written as text.
func ReadText(written string) (string, error) {
	value, err := ReadValue(written)
	if err != nil {
		return "", err
	}
	text, isText := value.(string)
	if !isText {
		return "", newSyntaxError("this argument must be a name in quotes")
	}
	return text, nil
}

// markEnd returns where a fault ends, so a fault over nothing still covers a character.
func (read *parser) markEnd(start int) int {
	if read.at > start {
		return read.at
	}
	if start < len(read.text) {
		return start + 1
	}
	return start
}

// skipBlank steps over the blanks and the comments.
func (read *parser) skipBlank() {
	for read.at < len(read.text) {
		if isBlank(read.text[read.at]) {
			read.at++
			continue
		}
		past, skipped := skipComment(read.text, read.at)
		if !skipped {
			return
		}
		read.at = past
	}
}

// readName reads one bare name.
func (read *parser) readName() string {
	start := read.at
	if read.at >= len(read.text) || !isNameStart(read.text[read.at]) {
		return ""
	}
	for read.at < len(read.text) && isNamePart(read.text[read.at]) {
		read.at++
	}
	return read.text[start:read.at]
}

// readCallArguments returns the text of each argument and where the call closes. It
// starts at the opening bracket.
func readCallArguments(text string, open int) ([]string, int, *SyntaxFault) {
	args := []string{}
	depth := 1
	index := open + 1
	argStart := index
	lastMeaning := byte('(')

	push := func(end int) {
		written := strings.TrimSpace(text[argStart:end])
		if written != "" || len(args) > 0 {
			args = append(args, written)
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
			depth--
			if depth == 0 {
				push(index)
				return args, index + 1, nil
			}
		case ',':
			if depth == 1 {
				push(index)
				argStart = index + 1
			}
		}
		if !isBlank(character) {
			lastMeaning = character
		}
		index++
	}
	return nil, 0, &SyntaxFault{
		Message: "missing closing parenthesis", Start: open, End: len(text),
	}
}

// skipComment returns where a comment ends, and whether one starts here.
func skipComment(text string, at int) (int, bool) {
	if at+1 >= len(text) || text[at] != '/' {
		return at, false
	}
	switch text[at+1] {
	case '/':
		index := at
		for index < len(text) && text[index] != '\n' {
			index++
		}
		return index, true
	case '*':
		closed := strings.Index(text[at+2:], "*/")
		if closed == -1 {
			return len(text), true
		}
		return at + closed + 4, true
	}
	return at, false
}

// skipText returns where a run of text ends, and whether one starts here: a quoted
// value, a comment, or a regular expression. The character before it decides whether a
// slash opens a regular expression or divides.
func skipText(text string, at int, lastMeaning byte) (int, bool) {
	character := text[at]
	if character == '"' || character == '\'' || character == '`' {
		return skipQuoted(text, at), true
	}
	if character != '/' {
		return at, false
	}
	if past, skipped := skipComment(text, at); skipped {
		return past, true
	}
	if !strings.ContainsRune(":,[({=", rune(lastMeaning)) {
		return at, false
	}
	return skipRegex(text, at), true
}

// skipQuoted returns where the quoted value that starts here ends.
func skipQuoted(text string, at int) int {
	quote := text[at]
	index := at + 1
	for index < len(text) {
		if text[index] == '\\' {
			index += 2
			continue
		}
		if text[index] == quote {
			return index + 1
		}
		index++
	}
	return len(text)
}

// skipRegex returns where the regular expression that starts here ends, with its flags.
func skipRegex(text string, at int) int {
	index := at + 1
	inClass := false
	for index < len(text) {
		character := text[index]
		if character == '\\' {
			index += 2
			continue
		}
		switch character {
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '\n':
			return index
		case '/':
			if !inClass {
				index++
				for index < len(text) && isNamePart(text[index]) {
					index++
				}
				return index
			}
		}
		index++
	}
	return len(text)
}
