package agent

import (
	"maps"
	"math"
	"slices"
	"strings"
)

// The arguments of a tool, written once and read two ways: as the JSON Schema a caller is
// given, and as the check the input goes through. An unknown field is refused rather than
// dropped, because a dropped field takes its default and the model reads that default as
// the value it asked for.

// The kinds of value a tool takes.
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
	// positive is true for a whole number that must be above zero.
	positive bool
}

// schemaDialect is the dialect of JSON Schema every caller is told the arguments are written
// in.
const schemaDialect = "http://json-schema.org/draft-07/schema#"

// largestWholeNumber is the highest whole number a JSON number carries without losing a
// digit, which is the upper limit of every count a tool takes.
const largestWholeNumber = 1<<53 - 1

// buildSchema writes the arguments as the JSON Schema a caller reads.
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

// BuildEmptySchema writes the schema of a tool that takes no arguments.
func BuildEmptySchema() map[string]any {
	return buildSchema(nil)
}

// ExtendSchema returns this schema with one more text field the caller must send, which is how
// the server adds the profile a call names. The schema of a definition is built once, so it is
// copied rather than written into.
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

// castProperties reads the properties of a schema, and returns nothing where it has none.
func castProperties(schema map[string]any) map[string]any {
	properties, is := schema["properties"].(map[string]any)
	if !is {
		return nil
	}
	return properties
}

// readInput checks the input against the arguments. It returns the values it read, and the
// reason it could not read them.
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

// readValue returns one value as the kind the argument asks for.
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

// readWholeNumber returns a number as a whole one. JSON carries every number as a float,
// so a value with a fraction is refused rather than cut.
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

// readText returns a text argument the input carried, and whether it carried one.
func readText(read map[string]any, name string) (string, bool) {
	written, is := read[name].(string)
	return written, is && written != ""
}

// readCount returns a whole number the input carried, and whether it carried one.
func readCount(read map[string]any, name string) (int, bool) {
	counted, is := read[name].(int)
	return counted, is
}

// readFlag returns a true or false the input carried, and whether it carried one.
func readFlag(read map[string]any, name string) (bool, bool) {
	answered, is := read[name].(bool)
	return answered, is
}
