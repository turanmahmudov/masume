package agent

import (
	"strings"
	"testing"
)

var scratchFields = []field{
	{name: "sql", kind: kindString, description: "the statement", required: true},
	{name: "limit", kind: kindInteger, description: "rows", positive: true},
	{name: "analyze", kind: kindBoolean, description: "measure it"},
}

func TestBuildSchema(t *testing.T) {
	schema := buildSchema(scratchFields)
	if schema["additionalProperties"] != false {
		t.Error("the schema allows an unknown field")
	}
	required, is := schema["required"].([]string)
	if !is || len(required) != 1 || required[0] != "sql" {
		t.Errorf("the needed fields are %v, wanted sql", schema["required"])
	}
	properties := schema["properties"].(map[string]any)
	limit := properties["limit"].(map[string]any)
	if limit["type"] != "integer" || limit["exclusiveMinimum"] != 0 {
		t.Errorf("the limit reads as %v", limit)
	}
}

func TestReadInputRefusesAnUnknownField(t *testing.T) {
	_, problem := readInput(scratchFields, map[string]any{"sql": "select 1", "sqll": "typo"})
	if !strings.Contains(problem, "sqll") {
		t.Errorf("the problem reads %q, wanted the unknown field named", problem)
	}
}

func TestReadInputWantsTheNeededField(t *testing.T) {
	_, problem := readInput(scratchFields, map[string]any{"limit": float64(5)})
	if !strings.Contains(problem, "sql") {
		t.Errorf("the problem reads %q, wanted the missing field named", problem)
	}
}

func TestReadInputChecksEachKind(t *testing.T) {
	cases := []struct {
		input  map[string]any
		reads  bool
		blames string
	}{
		{map[string]any{"sql": "select 1"}, true, ""},
		{map[string]any{"sql": 3}, false, "sql"},
		{map[string]any{"sql": "s", "limit": float64(2.5)}, false, "limit"},
		{map[string]any{"sql": "s", "limit": float64(0)}, false, "limit"},
		{map[string]any{"sql": "s", "limit": float64(10)}, true, ""},
		{map[string]any{"sql": "s", "analyze": "yes"}, false, "analyze"},
		{map[string]any{"sql": "s", "analyze": true}, true, ""},
		// A field a caller sent as null is read as one it did not send.
		{map[string]any{"sql": "s", "limit": nil}, true, ""},
	}
	for _, held := range cases {
		read, problem := readInput(scratchFields, held.input)
		if held.reads {
			if problem != "" {
				t.Errorf("%v was refused: %s", held.input, problem)
			}
			continue
		}
		if problem == "" {
			t.Errorf("%v was read, wanted a problem", held.input)
			continue
		}
		if !strings.Contains(problem, held.blames) {
			t.Errorf("%v gave %q, wanted %q named", held.input, problem, held.blames)
		}
		_ = read
	}
}

func TestReadValuesFromTheInput(t *testing.T) {
	read, problem := readInput(scratchFields, map[string]any{
		"sql": "select 1", "limit": float64(20), "analyze": true,
	})
	if problem != "" {
		t.Fatalf("the input was refused: %s", problem)
	}
	if written, given := readText(read, "sql"); !given || written != "select 1" {
		t.Errorf("the statement reads %q", written)
	}
	if counted, given := readCount(read, "limit"); !given || counted != 20 {
		t.Errorf("the limit reads %d", counted)
	}
	if answered, given := readFlag(read, "analyze"); !given || !answered {
		t.Errorf("the flag reads %v", answered)
	}
	if _, given := readText(read, "missing"); given {
		t.Error("a field that was not sent reads as given")
	}
}

func TestBuildSchemaNamesItsDialectAndItsLimits(t *testing.T) {
	schema := buildSchema(scratchFields)
	if schema["$schema"] != schemaDialect {
		t.Errorf("the schema names the dialect %v", schema["$schema"])
	}
	limit := schema["properties"].(map[string]any)["limit"].(map[string]any)
	if limit["maximum"] != largestWholeNumber {
		t.Errorf("the limit reads %v, wanted a highest whole number", limit["maximum"])
	}
	if len(buildSchema(nil)["properties"].(map[string]any)) != 0 {
		t.Error("a tool with no arguments has properties")
	}
	if _, named := buildSchema(nil)["required"]; named {
		t.Error("a tool with no arguments names a needed field")
	}
}

func TestExtendSchemaAddsTheProfile(t *testing.T) {
	schema := buildSchema(scratchFields)
	extended := ExtendSchema(schema, "profile", "the connection")

	properties := extended["properties"].(map[string]any)
	if len(properties) != len(scratchFields)+1 {
		t.Errorf("the schema holds %d fields", len(properties))
	}
	added := properties["profile"].(map[string]any)
	if added["type"] != kindString || added["description"] != "the connection" {
		t.Errorf("the added field reads %v", added)
	}
	required := extended["required"].([]string)
	if len(required) != 2 || required[0] != "sql" || required[1] != "profile" {
		t.Errorf("the needed fields are %v, wanted sql then profile", required)
	}

	// The schema of a definition is built once, so the one it was made from is untouched.
	if len(schema["properties"].(map[string]any)) != len(scratchFields) {
		t.Error("the schema it was made from grew a field")
	}
	if len(schema["required"].([]string)) != 1 {
		t.Error("the schema it was made from grew a needed field")
	}
}
