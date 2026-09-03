package core

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// This parser keeps the order of the members of every object. A map has no order, so a
// value parsed into a map and written again is in name order, and the fields of a value
// from the server must keep the order the server used.

// JSONValue is one JSON value: an object, an array, or a scalar in its written form.
type JSONValue struct {
	Members []JSONMember
	Items   []JSONValue
	// The written form of a scalar. It is empty for an object and for an array.
	Scalar   string
	IsObject bool
	IsArray  bool
}

// JSONMember is one member of an object: its name and its value.
type JSONMember struct {
	Name  string
	Value JSONValue
}

// ReadJSON parses JSON text. It returns false if the text is not exactly one JSON value.
func ReadJSON(text string) (JSONValue, bool) {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	value, err := readJSONValue(decoder)
	if err != nil {
		return JSONValue{}, false
	}
	// A second value after the first one means the text is not one JSON value.
	if _, err := decoder.Token(); err != io.EOF {
		return JSONValue{}, false
	}
	return value, true
}

func readJSONValue(decoder *json.Decoder) (JSONValue, error) {
	token, err := decoder.Token()
	if err != nil {
		return JSONValue{}, err
	}
	return readJSONFrom(decoder, token)
}

func readJSONFrom(decoder *json.Decoder, token json.Token) (JSONValue, error) {
	switch held := token.(type) {
	case json.Delim:
		if held == '{' {
			return readJSONObject(decoder)
		}
		if held == '[' {
			return readJSONArray(decoder)
		}
		return JSONValue{}, &json.SyntaxError{}
	case string:
		return JSONValue{Scalar: WriteJSONText(held)}, nil
	case json.Number:
		return JSONValue{Scalar: writeJSONNumber(held)}, nil
	case bool:
		if held {
			return JSONValue{Scalar: "true"}, nil
		}
		return JSONValue{Scalar: "false"}, nil
	case nil:
		return JSONValue{Scalar: "null"}, nil
	}
	return JSONValue{}, &json.SyntaxError{}
}

func readJSONObject(decoder *json.Decoder) (JSONValue, error) {
	value := JSONValue{IsObject: true, Members: []JSONMember{}}
	for {
		token, err := decoder.Token()
		if err != nil {
			return JSONValue{}, err
		}
		if end, isDelim := token.(json.Delim); isDelim && end == '}' {
			return value, nil
		}
		name, isName := token.(string)
		if !isName {
			return JSONValue{}, &json.SyntaxError{}
		}
		held, err := readJSONValue(decoder)
		if err != nil {
			return JSONValue{}, err
		}
		value.Members = append(value.Members, JSONMember{Name: name, Value: held})
	}
}

func readJSONArray(decoder *json.Decoder) (JSONValue, error) {
	value := JSONValue{IsArray: true, Items: []JSONValue{}}
	for {
		token, err := decoder.Token()
		if err != nil {
			return JSONValue{}, err
		}
		if end, isDelim := token.(json.Delim); isDelim && end == ']' {
			return value, nil
		}
		held, err := readJSONFrom(decoder, token)
		if err != nil {
			return JSONValue{}, err
		}
		value.Items = append(value.Items, held)
	}
}

// Write returns the value on one line, with no space between the parts.
func (value JSONValue) Write() string {
	written := &strings.Builder{}
	value.writeInto(written, "", "")
	return written.String()
}

// WriteIndented returns the value on several lines, with one indent step per level.
func (value JSONValue) WriteIndented(step string) string {
	written := &strings.Builder{}
	value.writeInto(written, step, "")
	return written.String()
}

func (value JSONValue) writeInto(written *strings.Builder, step, indent string) {
	switch {
	case value.IsObject:
		value.writeMembers(written, step, indent)
	case value.IsArray:
		value.writeItems(written, step, indent)
	default:
		written.WriteString(value.Scalar)
	}
}

func (value JSONValue) writeMembers(written *strings.Builder, step, indent string) {
	if len(value.Members) == 0 {
		written.WriteString("{}")
		return
	}
	inner := indent + step
	written.WriteString("{")
	for at, member := range value.Members {
		if at > 0 {
			written.WriteString(",")
		}
		writeJSONBreak(written, step, inner)
		written.WriteString(WriteJSONText(member.Name))
		written.WriteString(":")
		if step != "" {
			written.WriteString(" ")
		}
		member.Value.writeInto(written, step, inner)
	}
	writeJSONBreak(written, step, indent)
	written.WriteString("}")
}

func (value JSONValue) writeItems(written *strings.Builder, step, indent string) {
	if len(value.Items) == 0 {
		written.WriteString("[]")
		return
	}
	inner := indent + step
	written.WriteString("[")
	for at, item := range value.Items {
		if at > 0 {
			written.WriteString(",")
		}
		writeJSONBreak(written, step, inner)
		item.writeInto(written, step, inner)
	}
	writeJSONBreak(written, step, indent)
	written.WriteString("]")
}

func writeJSONBreak(written *strings.Builder, step, indent string) {
	if step == "" {
		return
	}
	written.WriteString("\n")
	written.WriteString(indent)
}

// WriteJSONText returns the text as a JSON string. It does not escape `<`, `>` and `&`,
// which the standard library escapes.
func WriteJSONText(text string) string {
	held := &bytes.Buffer{}
	encoder := json.NewEncoder(held)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(text); err != nil {
		return `""`
	}
	return strings.TrimSuffix(held.String(), "\n")
}

// WriteJSONValue returns one value as JSON. A bind value is displayed this way: a text
// keeps its quotes, and a line break becomes an escape and does not break the row.
func WriteJSONValue(value any) string {
	held := &bytes.Buffer{}
	encoder := json.NewEncoder(held)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return FormatCell(value, "")
	}
	return strings.TrimSuffix(held.String(), "\n")
}

// writeJSONNumber returns a number in the shortest form that parses back to the same
// value. An integer keeps all of its digits: a double holds 53 bits only, so a larger
// integer becomes a different number and a column of ids is displayed wrong.
func writeJSONNumber(number json.Number) string {
	written := number.String()
	if isWholeNumberText(written) {
		return written
	}
	held, err := number.Float64()
	if err != nil {
		return written
	}
	shortest, err := json.Marshal(held)
	if err != nil {
		return written
	}
	return string(shortest)
}

// isWholeNumberText is true for digits with an optional minus sign, which is already the
// shortest form of that number. JSON has no other sign, so all other text becomes a
// double.
func isWholeNumberText(written string) bool {
	digits := strings.TrimPrefix(written, "-")
	if digits == "" {
		return false
	}
	for _, character := range digits {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
