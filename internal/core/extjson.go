package core

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// A document holds values JSON has no room for: an identity, a moment, a decimal number.
// Extended JSON writes each of those as an object with one member whose name begins with a
// dollar. A reader that does not know the names draws the wrapper in place of the value, so
// they are read back here.

// The names of the types a document value can have, as the server names them. They are the
// names a MongoDB column already carries, so a value read out of a wrapper and a value read
// from a column are named the same way.
const (
	DocumentTypeObjectID  = "objectId"
	DocumentTypeString    = "string"
	DocumentTypeInt       = "int"
	DocumentTypeLong      = "long"
	DocumentTypeDouble    = "double"
	DocumentTypeDecimal   = "decimal"
	DocumentTypeBool      = "bool"
	DocumentTypeDate      = "datetime"
	DocumentTypeTimestamp = "timestamp"
	DocumentTypeObject    = "object"
	DocumentTypeArray     = "array"
	DocumentTypeBinary    = "binData"
	DocumentTypeRegex     = "regex"
	DocumentTypeCode      = "javascript"
	DocumentTypeSymbol    = "symbol"
	DocumentTypeMinKey    = "minKey"
	DocumentTypeMaxKey    = "maxKey"
	DocumentTypeUndefined = "undefined"
	DocumentTypePointer   = "dbPointer"
	DocumentTypeNull      = "null"
)

// DocumentScalar is a value an extended JSON wrapper holds: the value as a reader sees it,
// and the name of the type it was written for.
type DocumentScalar struct {
	Text string
	Type string
}

// ReadDocumentScalar returns the value an extended JSON wrapper holds. It answers false for
// every value that is not one, which is drawn as the JSON it already is.
func ReadDocumentScalar(value JSONValue) (DocumentScalar, bool) {
	if !value.IsObject || len(value.Members) == 0 {
		return DocumentScalar{}, false
	}
	name := value.Members[0].Name
	if !strings.HasPrefix(name, "$") {
		return DocumentScalar{}, false
	}
	held := value.Members[0].Value

	switch name {
	case "$oid":
		return readWrappedText(held, DocumentTypeObjectID)
	case "$symbol":
		return readWrappedText(held, DocumentTypeSymbol)
	case "$numberInt":
		return readWrappedNumber(held, DocumentTypeInt)
	case "$numberLong":
		return readWrappedNumber(held, DocumentTypeLong)
	case "$numberDouble":
		return readWrappedNumber(held, DocumentTypeDouble)
	case "$numberDecimal":
		return readWrappedNumber(held, DocumentTypeDecimal)
	case "$date":
		return readWrappedDate(held)
	case "$binary":
		return readWrappedBinary(held)
	case "$regularExpression":
		return readWrappedRegex(held)
	case "$timestamp":
		return readWrappedTimestamp(held)
	case "$code":
		return readWrappedText(held, DocumentTypeCode)
	case "$minKey":
		return DocumentScalar{Text: "MinKey", Type: DocumentTypeMinKey}, true
	case "$maxKey":
		return DocumentScalar{Text: "MaxKey", Type: DocumentTypeMaxKey}, true
	case "$undefined":
		return DocumentScalar{Text: NullText, Type: DocumentTypeUndefined}, true
	case "$dbPointer":
		return DocumentScalar{Text: held.Write(), Type: DocumentTypePointer}, true
	}
	return DocumentScalar{}, false
}

// readWrappedText reads a wrapper that holds one string, such as an identity.
func readWrappedText(held JSONValue, named string) (DocumentScalar, bool) {
	text, isText := ReadJSONTextValue(held.Scalar)
	if !isText {
		return DocumentScalar{}, false
	}
	return DocumentScalar{Text: text, Type: named}, true
}

// readWrappedNumber reads a wrapper that holds a number. Extended JSON writes the number as
// text, so that a reader of another language does not round it away.
func readWrappedNumber(held JSONValue, named string) (DocumentScalar, bool) {
	if text, isText := ReadJSONTextValue(held.Scalar); isText {
		return DocumentScalar{Text: text, Type: named}, true
	}
	if held.Scalar == "" || held.IsObject || held.IsArray {
		return DocumentScalar{}, false
	}
	return DocumentScalar{Text: held.Scalar, Type: named}, true
}

// readWrappedDate reads a moment. It is written as the milliseconds since the epoch where
// the writer kept every type, and as the moment itself where the writer wrote for a reader.
func readWrappedDate(held JSONValue) (DocumentScalar, bool) {
	if text, isText := ReadJSONTextValue(held.Scalar); isText {
		return DocumentScalar{Text: text, Type: DocumentTypeDate}, true
	}
	inner, isWrapped := ReadDocumentScalar(held)
	if !isWrapped {
		return DocumentScalar{}, false
	}
	milliseconds, err := strconv.ParseInt(inner.Text, 10, 64)
	if err != nil {
		return DocumentScalar{}, false
	}
	return DocumentScalar{
		Text: FormatClockTime(time.UnixMilli(milliseconds).UTC()), Type: DocumentTypeDate,
	}, true
}

// readWrappedBinary reads bytes, which are written as base64 beside the kind of bytes they
// are. The reader is shown how many bytes there are, because the base64 of a photograph is
// no more readable than the photograph.
func readWrappedBinary(held JSONValue) (DocumentScalar, bool) {
	if !held.IsObject {
		return DocumentScalar{}, false
	}
	written, _ := ReadJSONTextValue(findMember(held, "base64").Scalar)
	// Four characters of base64 carry three bytes, and the padding carries none.
	count := len(written) / 4 * 3
	count -= strings.Count(written, "=")
	written = "bytes"
	if count == 1 {
		written = "byte"
	}
	return DocumentScalar{
		Text: strconv.Itoa(max(count, 0)) + " " + written, Type: DocumentTypeBinary,
	}, true
}

// readWrappedRegex reads a pattern and the options beside it, and writes them the way a
// pattern is written.
func readWrappedRegex(held JSONValue) (DocumentScalar, bool) {
	if !held.IsObject {
		return DocumentScalar{}, false
	}
	pattern, _ := ReadJSONTextValue(findMember(held, "pattern").Scalar)
	options, _ := ReadJSONTextValue(findMember(held, "options").Scalar)
	return DocumentScalar{Text: "/" + pattern + "/" + options, Type: DocumentTypeRegex}, true
}

// readWrappedTimestamp reads the timestamp a replica set orders its work by, which is a
// second and a count inside that second.
func readWrappedTimestamp(held JSONValue) (DocumentScalar, bool) {
	if !held.IsObject {
		return DocumentScalar{}, false
	}
	seconds := findMember(held, "t").Scalar
	within := findMember(held, "i").Scalar
	if seconds == "" || within == "" {
		return DocumentScalar{}, false
	}
	return DocumentScalar{Text: seconds + ":" + within, Type: DocumentTypeTimestamp}, true
}

// findMember returns the member of that name, or nothing where the object has none.
func findMember(value JSONValue, name string) JSONValue {
	for _, member := range value.Members {
		if member.Name == name {
			return member.Value
		}
	}
	return JSONValue{}
}

// ReadJSONTextValue reads a JSON string back as the text it holds. It answers false where
// the value written is not a string.
func ReadJSONTextValue(written string) (string, bool) {
	if len(written) < 2 || written[0] != '"' {
		return "", false
	}
	var held string
	if err := json.Unmarshal([]byte(written), &held); err != nil {
		return "", false
	}
	return held, true
}

// ReadJSONScalarType names the type of a value that carries no wrapper, so every row of a
// document tree names a type whether the value was wrapped or not.
func ReadJSONScalarType(written string) string {
	switch {
	case written == "" || written == "null":
		return DocumentTypeNull
	case written == "true" || written == "false":
		return DocumentTypeBool
	case written[0] == '"':
		return DocumentTypeString
	}
	if strings.ContainsAny(written, ".eE") {
		return DocumentTypeDouble
	}
	return DocumentTypeLong
}

// ReadDocumentValue returns a value of a document as a reader sees it, with the name of its
// type. An object and an array are answered as they stand, because those are opened rather
// than drawn on one line.
func ReadDocumentValue(value JSONValue) DocumentScalar {
	if held, isWrapped := ReadDocumentScalar(value); isWrapped {
		return held
	}
	switch {
	case value.IsObject:
		return DocumentScalar{Type: DocumentTypeObject}
	case value.IsArray:
		return DocumentScalar{Type: DocumentTypeArray}
	}
	if text, isText := ReadJSONTextValue(value.Scalar); isText {
		return DocumentScalar{Text: text, Type: DocumentTypeString}
	}
	return DocumentScalar{Text: value.Scalar, Type: ReadJSONScalarType(value.Scalar)}
}
