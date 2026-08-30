package ai

import (
	"slices"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

// maxOtherSchemasNamed is the count above which the list of other databases is cut.
const maxOtherSchemasNamed = 300

// SchemaContextSource is what the schema context is built from.
type SchemaContextSource struct {
	DialectName string
	// DefaultSchema is the database the connection opened, on a server that can hold more.
	DefaultSchema string
	// Tables holds every relation this connection can see.
	Tables []db.TableRef
}

// BuildSchemaContext writes what the model is told first: the dialect and the databases. No
// table is named here, because the tools list them.
func BuildSchemaContext(source SchemaContextSource) string {
	held := map[string]bool{}
	otherSchemas := []string{}
	for _, table := range source.Tables {
		if table.Schema == source.DefaultSchema || held[table.Schema] {
			continue
		}
		held[table.Schema] = true
		otherSchemas = append(otherSchemas, table.Schema)
	}
	slices.Sort(otherSchemas)

	lines := []string{
		"Dialect: " + source.DialectName,
		"Connected database: " + source.DefaultSchema,
	}
	if len(otherSchemas) > 0 {
		named := otherSchemas
		left := 0
		if len(named) > maxOtherSchemasNamed {
			left = len(named) - maxOtherSchemasNamed
			named = named[:maxOtherSchemasNamed]
		}
		written := strings.Join(named, ", ")
		if left > 0 {
			written += ", and " + strconv.Itoa(left) + " more"
		}
		lines = append(lines, "",
			"Other databases this connection can also see, named only:", written)
	}
	return strings.Join(lines, "\n")
}
