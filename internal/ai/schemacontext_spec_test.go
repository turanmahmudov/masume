package ai_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/ai"
	"github.com/turanmahmudov/masume/internal/db"
)

func TestBuildSchemaContextDescribesNamespacesWithoutTableNames(t *testing.T) {
	schemaContext := ai.BuildSchemaContext(ai.SchemaContextSource{
		DialectName: "PostgreSQL", DefaultSchema: "public",
		Tables: []db.TableRef{
			{Schema: "public", Name: "orders"},
			{Schema: "reports", Name: "daily_sales"},
			{Schema: "audit", Name: "events"},
			{Schema: "audit", Name: "logins"},
		},
	})
	wanted := "Dialect: PostgreSQL\nDefault schema or database: public\n\n" +
		"Other schemas or databases in the loaded catalog:\naudit, reports"
	if schemaContext != wanted {
		t.Fatalf("schema context %q, want %q", schemaContext, wanted)
	}
}
