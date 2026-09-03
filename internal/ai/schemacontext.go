package ai

import (
	"slices"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

// maxOtherSchemasNamed is the number of other databases the list shows before it is
// truncated.
const maxOtherSchemasNamed = 300

// SchemaContextSource holds the data the schema context is built from.
type SchemaContextSource struct {
	DialectName string
	// DefaultSchema is the database of the connection, on a server that has several.
	DefaultSchema string
	// Tables holds every table this connection can see.
	Tables []db.TableRef
}

// BuildSchemaContext returns the first part of the prompt: the dialect and the databases. It
// names no table, because the tools list them.
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
