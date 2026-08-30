package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/turanmahmudov/masume/internal/db"
)

// readValueType returns the type of a value, for a column that no table gave a type.
func readValueType(value any) string {
	switch value.(type) {
	case nil:
		return ""
	case string:
		return "text"
	case bool, int64, int32, int:
		return "integer"
	case float64, float32:
		return "real"
	}
	return "blob"
}

// readColumns returns one result column per column of the statement. A name the
// result gives twice is numbered, so two columns of one name stay apart.
func readColumns(names []string, types []*sql.ColumnType, first []any) []db.ResultColumn {
	seen := map[string]int{}
	for _, name := range names {
		seen[name]++
	}
	named := true
	for _, count := range seen {
		if count > 1 {
			named = false
		}
	}

	columns := make([]db.ResultColumn, 0, len(names))
	for at, name := range names {
		written := name
		if !named {
			written = fmt.Sprintf("column_%d", at+1)
		}
		dataType := ""
		if at < len(types) && types[at] != nil {
			dataType = strings.ToLower(types[at].DatabaseTypeName())
		}
		if dataType == "" && at < len(first) {
			dataType = readValueType(first[at])
		}
		columns = append(columns, db.ResultColumn{Name: written, DataType: dataType})
	}
	return columns
}
