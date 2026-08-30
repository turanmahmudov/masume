package mongo

import (
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The shell writes a document the way JavaScript writes an object: a bare key, a single
// quote, a regular expression between slashes, and a helper such as ObjectId. The driver
// reads extended JSON only, so the text is written again here before it is read.

// shellHelpers name the helper functions of the shell, and the extended JSON field each
// one becomes. A helper that takes a number keeps the number as text, which is what
// extended JSON asks for.
var shellHelpers = map[string]string{
	"ObjectId":      "$oid",
	"ISODate":       "$date",
	"Date":          "$date",
	"NumberLong":    "$numberLong",
	"NumberInt":     "$numberInt",
	"NumberDouble":  "$numberDouble",
	"NumberDecimal": "$numberDecimal",
	"UUID":          "$uuid",
}

// jsonWords are the three bare words that stay bare, because JSON reads them itself.
var jsonWords = map[string]bool{"true": true, "false": true, "null": true}

// ReadValue reads one value of a statement: a document, an array, or a single value. It
// is wrapped in a document first, because extended JSON is read from a document only.
func ReadValue(written string) (any, error) {
	relaxed, err := RelaxValue(written)
	if err != nil {
		return nil, err
	}
	var wrapper bson.D
	if unmarshalErr := bson.UnmarshalExtJSON(
		[]byte(`{"v":`+relaxed+`}`), false, &wrapper); unmarshalErr != nil {
		return nil, unmarshalErr
	}
	if len(wrapper) == 0 {
		return nil, nil
	}
	return wrapper[0].Value, nil
}

// ReadDocument reads one value and returns it as a document.
func ReadDocument(written string) (bson.D, error) {
	value, err := ReadValue(written)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return bson.D{}, nil
	}
	document, isDocument := value.(bson.D)
	if !isDocument {
		return nil, newSyntaxError("this argument is not a document")
	}
	return document, nil
}

// ReadArray reads one value and returns it as an array, which is what a pipeline is.
func ReadArray(written string) (bson.A, error) {
	value, err := ReadValue(written)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return bson.A{}, nil
	}
	array, isArray := value.(bson.A)
	if !isArray {
		return nil, newSyntaxError("this argument is not an array")
	}
	return array, nil
}

// relaxer writes the shell form of a value again as extended JSON.
type relaxer struct {
	source string
	at     int
	built  strings.Builder
}

// RelaxValue writes the shell form of a value again as the extended JSON the driver
// reads. Text that is already extended JSON passes through unchanged.
func RelaxValue(written string) (string, error) {
	if strings.TrimSpace(written) == "" {
		return "null", nil
	}
	relax := &relaxer{source: written}
	if err := relax.rewrite(); err != nil {
		return "", err
	}
	return relax.built.String(), nil
}

// rewrite walks the whole value once, writing each part in its JSON form.
func (relax *relaxer) rewrite() error {
	for relax.at < len(relax.source) {
		character := relax.source[relax.at]
		switch {
		case isBlank(character):
			relax.built.WriteByte(' ')
			relax.at++
		case character == '/':
			if err := relax.writeSlash(); err != nil {
				return err
			}
		case character == '"' || character == '\'':
			if err := relax.writeString(); err != nil {
				return err
			}
		case character == ',':
			relax.writeComma()
		case isDigit(character):
			relax.writeNumber()
		case isNameStart(character):
			if err := relax.writeWord(); err != nil {
				return err
			}
		default:
			relax.built.WriteByte(character)
			relax.at++
		}
	}
	return nil
}

// writeNumber writes a number as it stands, so the exponent of one is not read as a
// bare word.
func (relax *relaxer) writeNumber() {
	start := relax.at
	for relax.at < len(relax.source) {
		character := relax.source[relax.at]
		if isDigit(character) || character == '.' {
			relax.at++
			continue
		}
		if character == 'e' || character == 'E' {
			relax.at++
			if relax.at < len(relax.source) &&
				(relax.source[relax.at] == '+' || relax.source[relax.at] == '-') {
				relax.at++
			}
			continue
		}
		break
	}
	relax.built.WriteString(relax.source[start:relax.at])
}

// writeComma drops a comma that stands before a closing bracket, which the shell allows
// and JSON does not.
func (relax *relaxer) writeComma() {
	relax.at++
	next := relax.findNextMeaning()
	if next < len(relax.source) &&
		(relax.source[next] == '}' || relax.source[next] == ']') {
		return
	}
	relax.built.WriteByte(',')
}

// writeSlash writes a comment away, or writes a regular expression as extended JSON.
func (relax *relaxer) writeSlash() error {
	if relax.at+1 < len(relax.source) {
		switch relax.source[relax.at+1] {
		case '/':
			for relax.at < len(relax.source) && relax.source[relax.at] != '\n' {
				relax.at++
			}
			return nil
		case '*':
			closed := strings.Index(relax.source[relax.at+2:], "*/")
			if closed == -1 {
				return newSyntaxError("this comment never closes")
			}
			relax.at += closed + 4
			return nil
		}
	}
	return relax.writeRegex()
}

// writeRegex writes /pattern/flags as the extended JSON form of a regular expression.
func (relax *relaxer) writeRegex() error {
	index := relax.at + 1
	inClass := false
	for index < len(relax.source) {
		character := relax.source[index]
		if character == '\\' {
			index += 2
			continue
		}
		if character == '[' {
			inClass = true
		}
		if character == ']' {
			inClass = false
		}
		if character == '/' && !inClass {
			break
		}
		if character == '\n' {
			return newSyntaxError("this regular expression never closes")
		}
		index++
	}
	if index >= len(relax.source) {
		return newSyntaxError("this regular expression never closes")
	}

	pattern := relax.source[relax.at+1 : index]
	index++
	flagsStart := index
	for index < len(relax.source) && isNamePart(relax.source[index]) {
		index++
	}

	relax.built.WriteString(`{"$regularExpression":{"pattern":`)
	relax.built.WriteString(writeJSONString(pattern))
	relax.built.WriteString(`,"options":`)
	relax.built.WriteString(writeJSONString(relax.source[flagsStart:index]))
	relax.built.WriteString("}}")
	relax.at = index
	return nil
}

// writeString writes a quoted text as a JSON string, whichever quote mark holds it.
func (relax *relaxer) writeString() error {
	quote := relax.source[relax.at]
	index := relax.at + 1
	var text strings.Builder

	for index < len(relax.source) {
		character := relax.source[index]
		if character == '\\' && index+1 < len(relax.source) {
			text.WriteString(readEscape(relax.source[index+1]))
			index += 2
			continue
		}
		if character == quote {
			relax.built.WriteString(writeJSONString(text.String()))
			relax.at = index + 1
			return nil
		}
		text.WriteByte(character)
		index++
	}
	return newSyntaxError("this quote never closes")
}

// readEscape returns what the shell escape stands for.
func readEscape(marked byte) string {
	switch marked {
	case 'n':
		return "\n"
	case 't':
		return "\t"
	case 'r':
		return "\r"
	}
	return string(marked)
}

// writeWord writes a bare word: a helper of the shell, a word JSON reads itself, a key,
// or a value that is quoted so it reads as text.
func (relax *relaxer) writeWord() error {
	start := relax.at
	for relax.at < len(relax.source) && isNamePart(relax.source[relax.at]) {
		relax.at++
	}
	word := relax.source[start:relax.at]

	next := relax.findNextMeaning()
	if field, isHelper := shellHelpers[word]; isHelper &&
		next < len(relax.source) && relax.source[next] == '(' {
		return relax.writeHelper(field, next)
	}
	if jsonWords[word] {
		relax.built.WriteString(word)
		return nil
	}
	relax.built.WriteString(writeJSONString(word))
	return nil
}

// writeHelper writes ObjectId("…") and the other helpers of the shell as the extended
// JSON field each one stands for.
func (relax *relaxer) writeHelper(field string, open int) error {
	closed := strings.IndexByte(relax.source[open:], ')')
	if closed == -1 {
		return newSyntaxError("this call never closes")
	}
	inside := strings.TrimSpace(relax.source[open+1 : open+closed])
	relax.at = open + closed + 1

	// The helper of a number is written with the number as text, which is the form
	// extended JSON reads.
	unquoted := strings.Trim(inside, `"'`)
	if inside == "" && field == "$date" {
		return newSyntaxError("Date needs the date it stands for")
	}
	relax.built.WriteString(`{"` + field + `":` + writeJSONString(unquoted) + "}")
	return nil
}

// findNextMeaning returns where the next character that is not a blank or a comment
// stands.
func (relax *relaxer) findNextMeaning() int {
	index := relax.at
	for index < len(relax.source) {
		if isBlank(relax.source[index]) {
			index++
			continue
		}
		if relax.source[index] == '/' && index+1 < len(relax.source) {
			if relax.source[index+1] == '/' {
				for index < len(relax.source) && relax.source[index] != '\n' {
					index++
				}
				continue
			}
			if relax.source[index+1] == '*' {
				closed := strings.Index(relax.source[index+2:], "*/")
				if closed == -1 {
					return len(relax.source)
				}
				index += closed + 4
				continue
			}
		}
		return index
	}
	return len(relax.source)
}

// writeJSONString writes text as a JSON string.
func writeJSONString(text string) string {
	return strconv.Quote(text)
}

func isBlank(character byte) bool {
	return character == ' ' || character == '\t' || character == '\n' ||
		character == '\r' || character == '\v' || character == '\f'
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}

func isNameStart(character byte) bool {
	return character == '_' || character == '$' ||
		(character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z')
}

func isNamePart(character byte) bool {
	return isNameStart(character) || (character >= '0' && character <= '9')
}
