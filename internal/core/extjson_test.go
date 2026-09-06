package core

import "testing"

// A file a reader outside MongoDB opens holds plain JSON, so every wrapper is gone.
func TestRelaxDocumentJSONUnwrapsEveryValue(t *testing.T) {
	for _, held := range []struct{ name, text, want string }{
		{"a nested array of numbers",
			`{"nums":[{"$numberInt":"1"},{"$numberInt":"2"}]}`, `{"nums":[1,2]}`},
		{"a nested document",
			`{"deep":{"inner":[{"$numberLong":"3"}]}}`, `{"deep":{"inner":[3]}}`},
		{"an id", `{"id":{"$oid":"507f1f77bcf86cd799439011"}}`,
			`{"id":"507f1f77bcf86cd799439011"}`},
		{"a decimal", `{"d":{"$numberDecimal":"1.50"}}`, `{"d":1.50}`},
		{"plain JSON", `{"a":[1,"b",true,null]}`, `{"a":[1,"b",true,null]}`},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered, isJSON := RelaxDocumentJSON(held.text)
			if !isJSON {
				t.Fatalf("%q was not read as JSON", held.text)
			}
			if answered != held.want {
				t.Errorf("%q became %q, wanted %q", held.text, answered, held.want)
			}
		})
	}
}

func TestRelaxDocumentJSONReportsTextThatIsNoValue(t *testing.T) {
	if _, isJSON := RelaxDocumentJSON("(not json)"); isJSON {
		t.Error("text that is no JSON value was read as one")
	}
}
