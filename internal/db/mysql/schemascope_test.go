package mysql

import (
	"strings"
	"testing"
)

// A profile that names a database reads that database alone, so the statement carries the
// name as a parameter and not as text of its own.
func TestTheCatalogStatementsKeepToTheDatabaseOfTheProfile(t *testing.T) {
	for _, one := range []struct {
		name   string
		build  func(string) (string, []any)
		column string
	}{
		{"schemas", buildMysqlSchemasSQL, "schema_name = ?"},
		{"tables", buildMysqlTablesSQL, "table_schema = ?"},
		{"routines", buildMysqlRoutinesSQL, "routine_schema = ?"},
		{"triggers", buildMysqlTriggersSQL, "trigger_schema = ?"},
		{"relationships", buildMysqlRelationshipsSQL, "k.table_schema = ?"},
	} {
		statement, params := one.build("shop")
		if !strings.Contains(statement, one.column) {
			t.Errorf("the %s statement holds no filter:\n%s", one.name, statement)
		}
		if len(params) != 1 || params[0] != "shop" {
			t.Errorf("the %s statement reads %v, wanted the database", one.name, params)
		}

		statement, params = one.build("")
		if strings.Contains(statement, one.column) {
			t.Errorf("the %s statement of a whole server holds a filter:\n%s",
				one.name, statement)
		}
		if len(params) != 0 {
			t.Errorf("the %s statement of a whole server reads %v", one.name, params)
		}
	}
}
