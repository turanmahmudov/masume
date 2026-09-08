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

// readColumns returns result columns with unique names. Repeated names receive numeric suffixes.
func readColumns(names []string, types []*sql.ColumnType, first []any) []db.ResultColumn {
	used := map[string]bool{}
	columns := make([]db.ResultColumn, 0, len(names))
	for at, name := range names {
		written := name
		if written == "" {
			written = fmt.Sprintf("column_%d", at+1)
		}
		for suffix := 2; used[written]; suffix++ {
			written = fmt.Sprintf("%s_%d", name, suffix)
		}
		used[written] = true
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
