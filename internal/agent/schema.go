package agent

import (
	"maps"
	"math"
	"slices"
	"strings"
)

// The arguments of a tool, defined one time and used two ways: as the JSON Schema given to
// a caller, and as the validation of the input. An unknown field gives an error and is not
// ignored, because an ignored field takes its default and the model reads that default as
// the value it sent.

// The kinds of value a tool accepts.
const (
	kindString  = "string"
	kindInteger = "integer"
	kindBoolean = "boolean"
)

// field is one argument of a tool.
type field struct {
	name        string
	kind        string
	description string
	required    bool
	// positive is true for an integer that must be above zero.
	positive bool
}

// schemaDialect is the JSON Schema dialect reported to every caller.
const schemaDialect = "http://json-schema.org/draft-07/schema#"

// largestWholeNumber is the highest integer a JSON number holds without a loss of
// precision. It is the maximum of every count a tool accepts.
const largestWholeNumber = 1<<53 - 1

// buildSchema converts the arguments into the JSON Schema a caller reads.
func buildSchema(fields []field) map[string]any {
	properties := map[string]any{}
	required := []string{}
	for _, held := range fields {
		property := map[string]any{"type": held.kind, "description": held.description}
		if held.kind == kindInteger {
			property["maximum"] = largestWholeNumber
		}
		if held.positive {
			property["exclusiveMinimum"] = 0
		}
		properties[held.name] = property
		if held.required {
			required = append(required, held.name)
		}
	}
	schema := map[string]any{
		"$schema": schemaDialect, "type": "object", "properties": properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// BuildEmptySchema returns the schema of a tool without arguments.
func BuildEmptySchema() map[string]any {
	return buildSchema(nil)
}

// ExtendSchema returns the schema with one more required text field. The server uses it to
// add the profile of a call. The schema of a definition is built one time, so this function
// copies it and does not modify it.
func ExtendSchema(schema map[string]any, name, description string) map[string]any {
	extended := map[string]any{}
	maps.Copy(extended, schema)

	properties := map[string]any{name: map[string]any{
		"type": kindString, "description": description,
	}}
	maps.Copy(properties, castProperties(schema))
	extended["properties"] = properties

	required, _ := schema["required"].([]string)
	extended["required"] = append(append([]string{}, required...), name)
	return extended
}

// castProperties returns the properties of a schema, and false if it has none.
func castProperties(schema map[string]any) map[string]any {
	properties, is := schema["properties"].(map[string]any)
	if !is {
		return nil
	}
	return properties
}

// readInput validates the input against the arguments. It returns the values, or the reason
// the input is invalid.
func readInput(fields []field, input map[string]any) (map[string]any, string) {
	known := map[string]field{}
	for _, held := range fields {
		known[held.name] = held
	}

	problems := []string{}
	for name := range input {
		if _, allowed := known[name]; !allowed {
			problems = append(problems, name+": this tool takes no such field")
		}
	}
	slices.Sort(problems)

	read := map[string]any{}
	for _, held := range fields {
		value, given := input[held.name]
		if !given || value == nil {
			if held.required {
				problems = append(problems, held.name+": this field is needed")
			}
			continue
		}
		kept, problem := readValue(held, value)
		if problem != "" {
			problems = append(problems, held.name+": "+problem)
			continue
		}
		read[held.name] = kept
	}

	if len(problems) > 0 {
		return nil, "this call cannot be read: " + strings.Join(problems, "; ")
	}
	return read, ""
}

// readValue returns one value in the kind the argument defines.
func readValue(held field, value any) (any, string) {
	switch held.kind {
	case kindString:
		written, is := value.(string)
		if !is {
			return nil, "a text is wanted"
		}
		return written, ""
	case kindBoolean:
		answered, is := value.(bool)
		if !is {
			return nil, "true or false is wanted"
		}
		return answered, ""
	case kindInteger:
		counted, problem := readWholeNumber(value)
		if problem != "" {
			return nil, problem
		}
		if held.positive && counted <= 0 {
			return nil, "a number above zero is wanted"
		}
		return counted, ""
	}
	return nil, "this field cannot be read"
}

// readWholeNumber returns a number as an integer. JSON sends every number as a float, so a
// value with a fraction gives an error and is not truncated.
func readWholeNumber(value any) (int, string) {
	switch held := value.(type) {
	case float64:
		if held != math.Trunc(held) {
			return 0, "a whole number is wanted"
		}
		return int(held), ""
	case int:
		return held, ""
	case int64:
		return int(held), ""
	}
	return 0, "a whole number is wanted"
}

// readText returns a text argument of the input, and whether the input has one.
func readText(read map[string]any, name string) (string, bool) {
	written, is := read[name].(string)
	return written, is && written != ""
}

// readCount returns an integer argument of the input, and whether the input has one.
func readCount(read map[string]any, name string) (int, bool) {
	counted, is := read[name].(int)
	return counted, is
}

// readFlag returns a boolean argument of the input, and whether the input has one.
func readFlag(read map[string]any, name string) (bool, bool) {
	answered, is := read[name].(bool)
	return answered, is
}
