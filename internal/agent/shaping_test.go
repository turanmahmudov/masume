package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

// A value goes to the model as JSON. A number sent as a number can be compared and used in
// arithmetic. A number sent as text cannot, and the model would put it in quotes in the next
// statement.
func TestDescribeCellForModelKeepsTheTypeOfAValue(t *testing.T) {
	for _, held := range []struct {
		name  string
		value any
		want  any
	}{
		{"nothing", nil, nil},
		{"text", "ada", "ada"},
		{"a whole number", int64(42), int64(42)},
		{"a number with a fraction", 12.5, 12.5},
		{"true", true, true},
		{"a small number", int32(7), int32(7)},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := describeCellForModel(held.value, "text"); answered != held.want {
				t.Errorf("%v goes to the model as %#v, wanted %#v",
					held.value, answered, held.want)
			}
		})
	}
}

// A type that JSON cannot express becomes text, because the model needs a readable value and
// a Go value would not survive the encoding.
func TestDescribeCellForModelWritesWhatJsonCannotHold(t *testing.T) {
	at := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	answered := describeCellForModel(at, "timestamptz")
	if _, isText := answered.(string); !isText {
		t.Errorf("a moment goes to the model as %#v, wanted text", answered)
	}

	// Whatever the result is, it must encode, or the whole answer is lost.
	if _, err := json.Marshal(answered); err != nil {
		t.Errorf("the value does not encode: %v", err)
	}
}

// The answer holds the columns with their types and the rows, so the model can write the
// next statement without another request.
func TestDescribeResultForModelHoldsTheColumnsAndTheRows(t *testing.T) {
	answered := describeResultForModel(db.QueryResult{
		Columns: []db.ResultColumn{
			{Name: "id", DataType: "integer"},
			{Name: "customer", DataType: "text"},
		},
		Rows:    [][]any{{int64(1), "ada"}, {int64(2), nil}},
		Elapsed: 5 * time.Millisecond,
	})

	written, err := json.Marshal(answered)
	if err != nil {
		t.Fatalf("the answer does not encode: %v", err)
	}

	var read map[string]any
	if err := json.Unmarshal(written, &read); err != nil {
		t.Fatalf("the answer does not read back: %v", err)
	}

	columns, is := read["columns"].([]any)
	if !is || len(columns) != 2 {
		t.Fatalf("the answer holds %v columns", read["columns"])
	}
	first, is := columns[0].(map[string]any)
	if !is || first["name"] != "id" || first["type"] != "integer" {
		t.Errorf("the first column reads %v", columns[0])
	}

	rows, is := read["rows"].([]any)
	if !is || len(rows) != 2 {
		t.Fatalf("the answer holds %v rows", read["rows"])
	}
	// A null stays a null, so the model does not read it as the text "null".
	second, is := rows[1].([]any)
	if !is || second[1] != nil {
		t.Errorf("the null of the second row reads %#v", rows[1])
	}
}

// A truncated read must report the truncation, or the model treats the page as the whole
// table and returns a wrong total.
func TestDescribeResultForModelReportsAReadThatWasCut(t *testing.T) {
	held := describeResultForModel(db.QueryResult{
		Columns:   []db.ResultColumn{{Name: "id"}},
		Rows:      [][]any{{int64(1)}},
		Truncated: true,
	})
	written, err := json.Marshal(held)
	if err != nil {
		t.Fatalf("the answer does not encode: %v", err)
	}
	if !contains(string(written), "truncated") {
		t.Errorf("a read that was cut says nothing about it:\n%s", written)
	}
}

// A column without a default is written as null, because an empty text is a valid default
// and the model must see the difference.
func TestDescribeColumnForModelTellsNoDefaultFromAnEmptyOne(t *testing.T) {
	none := describeColumnForModel(db.ColumnDetail{Name: "id", HasDefault: false})
	if none["default"] != nil {
		t.Errorf("a column with no default reads %#v", none["default"])
	}

	empty := describeColumnForModel(db.ColumnDetail{
		Name: "customer", HasDefault: true, DefaultValue: "",
	})
	if empty["default"] == nil {
		t.Error("a column whose default is an empty text reads as having none")
	}
}

// The values of an enum column are included, so the model can write
// `where status = 'shipped'` without a read from the server.
func TestDescribeColumnForModelGivesTheValuesOfAnEnum(t *testing.T) {
	held := describeColumnForModel(db.ColumnDetail{
		Name: "status", DataType: "order_status",
		Choices: []string{"pending", "shipped"},
	})
	choices, is := held["choices"].([]string)
	if !is || len(choices) != 2 {
		t.Fatalf("the values read %#v", held["choices"])
	}

	// A column that accepts any value has no list, and not an empty list, which the model
	// would read as a column that accepts no value.
	plain := describeColumnForModel(db.ColumnDetail{Name: "customer", DataType: "text"})
	if _, there := plain["choices"]; there {
		t.Errorf("a plain column names values: %#v", plain["choices"])
	}
}

// A foreign key names the target table with its schema, because the model writes a join
// from it and a name alone can exist in two schemas.
func TestDescribeForeignKeyForModelNamesTheTargetWithItsSchema(t *testing.T) {
	held := describeForeignKeyForModel(query.ForeignKey{
		Columns:       []string{"customer_id"},
		TargetSchema:  "public",
		TargetTable:   "customers",
		TargetColumns: []string{"id"},
	})
	if held["targetTable"] != "public.customers" {
		t.Errorf("the target reads %#v, wanted the schema and the name", held["targetTable"])
	}
	if _, err := json.Marshal(held); err != nil {
		t.Errorf("the key does not encode: %v", err)
	}
}

func contains(written, wanted string) bool {
	return len(written) >= len(wanted) && indexOf(written, wanted) >= 0
}

func indexOf(written, wanted string) int {
	for at := 0; at+len(wanted) <= len(written); at++ {
		if written[at:at+len(wanted)] == wanted {
			return at
		}
	}
	return -1
}
