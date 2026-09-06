package ai

import (
	"slices"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

// maxOtherSchemasNamed is the maximum number of other schemas or databases in the summary.
const maxOtherSchemasNamed = 300

// SchemaContextSource is the metadata for the schema summary.
type SchemaContextSource struct {
	DialectName string
	// DefaultSchema is the default schema or database, depending on the engine.
	DefaultSchema string
	// Tables is the loaded table catalog.
	Tables []db.TableRef
}

// BuildSchemaContext returns the dialect and schema or database names without table names.
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
		"Default schema or database: " + source.DefaultSchema,
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
			"Other schemas or databases in the loaded catalog:", written)
	}
	return strings.Join(lines, "\n")
}
