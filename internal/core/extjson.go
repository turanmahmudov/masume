package core

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Extended JSON wrappers preserve BSON types such as ObjectId, timestamps, and decimals.

// Document values and MongoDB columns use these type names.
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

// DocumentScalar is the content of an extended JSON wrapper: the value in display form,
// and the name of its type.
type DocumentScalar struct {
	Text string
	Type string
}

// ReadDocumentScalar returns the content of an extended JSON wrapper. It returns false for
// any other value, which is displayed as plain JSON.
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

// readWrappedText reads a wrapper that holds one string, such as an id.
func readWrappedText(held JSONValue, named string) (DocumentScalar, bool) {
	text, isText := ReadJSONTextValue(held.Scalar)
	if !isText {
		return DocumentScalar{}, false
	}
	return DocumentScalar{Text: text, Type: named}, true
}

// readWrappedNumber reads an Extended JSON number from a string or scalar.
func readWrappedNumber(held JSONValue, named string) (DocumentScalar, bool) {
	if text, isText := ReadJSONTextValue(held.Scalar); isText {
		return DocumentScalar{Text: text, Type: named}, true
	}
	if held.Scalar == "" || held.IsObject || held.IsArray {
		return DocumentScalar{}, false
	}
	return DocumentScalar{Text: held.Scalar, Type: named}, true
}

// readWrappedDate reads a timestamp. A writer in strict mode writes the milliseconds since
// the epoch, and a writer in relaxed mode writes the date and time.
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

// readWrappedBinary returns the decoded byte count for an Extended JSON binary value.
func readWrappedBinary(held JSONValue) (DocumentScalar, bool) {
	if !held.IsObject {
		return DocumentScalar{}, false
	}
	written, _ := ReadJSONTextValue(findMember(held, "base64").Scalar)
	// Four base64 characters hold three bytes, and the padding holds none.
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

// readWrappedRegex reads a pattern with its options and returns them in the usual pattern
// form.
func readWrappedRegex(held JSONValue) (DocumentScalar, bool) {
	if !held.IsObject {
		return DocumentScalar{}, false
	}
	pattern, _ := ReadJSONTextValue(findMember(held, "pattern").Scalar)
	options, _ := ReadJSONTextValue(findMember(held, "options").Scalar)
	return DocumentScalar{Text: "/" + pattern + "/" + options, Type: DocumentTypeRegex}, true
}

// readWrappedTimestamp reads the timestamp a replica set uses to order its operations. It
// is a second and a counter inside that second.
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

// findMember returns the named member value, or an empty JSONValue if absent.
func findMember(value JSONValue, name string) JSONValue {
	for _, member := range value.Members {
		if member.Name == name {
			return member.Value
		}
	}
	return JSONValue{}
}

// ReadJSONTextValue returns the text of a JSON string. It returns false if the written
// value is not a string.
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

// ReadJSONScalarType returns the type of an unwrapped JSON scalar.
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

// ReadDocumentValue returns display text and type. Objects and arrays return only their type.
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

// RelaxDocumentJSON replaces Extended JSON wrappers with display values. Invalid JSON returns false.
func RelaxDocumentJSON(text string) (string, bool) {
	value, isJSON := ReadJSON(text)
	if !isJSON {
		return "", false
	}
	return relaxJSONValue(value).Write(), true
}

func relaxJSONValue(value JSONValue) JSONValue {
	if held, isWrapped := ReadDocumentScalar(value); isWrapped {
		return JSONValue{Scalar: writeRelaxedScalar(held)}
	}
	switch {
	case value.IsObject:
		members := make([]JSONMember, 0, len(value.Members))
		for _, member := range value.Members {
			members = append(members,
				JSONMember{Name: member.Name, Value: relaxJSONValue(member.Value)})
		}
		return JSONValue{IsObject: true, Members: members}
	case value.IsArray:
		items := make([]JSONValue, 0, len(value.Items))
		for _, item := range value.Items {
			items = append(items, relaxJSONValue(item))
		}
		return JSONValue{IsArray: true, Items: items}
	}
	return value
}

// writeRelaxedScalar returns the unwrapped value in its JSON form. A number stays a number,
// and every other type becomes a string.
func writeRelaxedScalar(held DocumentScalar) string {
	switch held.Type {
	case DocumentTypeInt, DocumentTypeLong, DocumentTypeDouble, DocumentTypeDecimal:
		if _, err := strconv.ParseFloat(held.Text, 64); err == nil {
			return held.Text
		}
	case DocumentTypeUndefined:
		return "null"
	}
	return WriteJSONText(held.Text)
}
