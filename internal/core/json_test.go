package core

import "testing"

func TestReadJsonAgainstTheOtherClient(t *testing.T) {
	for _, held := range []struct{ text, compact, pretty string }{
		{`{"weight":1,"fragile":false}`, `{"weight":1,"fragile":false}`,
			"{\n  \"weight\": 1,\n  \"fragile\": false\n}"},
		{`{"b":1,"a":2}`, `{"b":1,"a":2}`, "{\n  \"b\": 1,\n  \"a\": 2\n}"},
		{`{"a": 1.0, "b": 1e2, "c": 100}`, `{"a":1,"b":100,"c":100}`,
			"{\n  \"a\": 1,\n  \"b\": 100,\n  \"c\": 100\n}"},
		{`{"a":"<b>&'"}`, `{"a":"<b>&'"}`, "{\n  \"a\": \"<b>&'\"\n}"},
		{`{"a":{"d":1,"c":[1,{"z":1,"y":2}]}}`, `{"a":{"d":1,"c":[1,{"z":1,"y":2}]}}`,
			"{\n  \"a\": {\n    \"d\": 1,\n    \"c\": [\n      1,\n      {\n        \"z\": 1,\n        \"y\": 2\n      }\n    ]\n  }\n}"},
		{`[]`, `[]`, `[]`},
		{`{}`, `{}`, `{}`},
		{`"text"`, `"text"`, `"text"`},
		{`3`, `3`, `3`},
		{`true`, `true`, `true`},
		{`null`, `null`, `null`},
		// An integer keeps all of its digits. A double holds 53 bits only, so a large
		// id read through a double becomes a different id.
		{`{"a":12345678901234567890}`, `{"a":12345678901234567890}`,
			"{\n  \"a\": 12345678901234567890\n}"},
		{`{"a":-9007199254740993}`, `{"a":-9007199254740993}`,
			"{\n  \"a\": -9007199254740993\n}"},
		{`{"a":"A\n\t"}`, `{"a":"A\n\t"}`, "{\n  \"a\": \"A\\n\\t\"\n}"},
	} {
		value, isJSON := ReadJSON(held.text)
		if !isJSON {
			t.Errorf("%s is not read as JSON", held.text)
			continue
		}
		if written := value.Write(); written != held.compact {
			t.Errorf("%s writes %s, wanted %s", held.text, written, held.compact)
		}
		if written := value.WriteIndented("  "); written != held.pretty {
			t.Errorf("%s indents %q, wanted %q", held.text, written, held.pretty)
		}
	}
}

func TestReadJsonRefusesWhatIsNotOneValue(t *testing.T) {
	for _, text := range []string{"", "not json", "{", `{"a":1} {"b":2}`, `{"a":}`} {
		if _, isJSON := ReadJSON(text); isJSON {
			t.Errorf("%q is read as JSON", text)
		}
	}
}
