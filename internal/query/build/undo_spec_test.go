package build_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query/build"
)

func TestBuildUndoUpdateBindsEveryValue(t *testing.T) {
	target := buildOrdersTarget("id")
	row := []any{int64(7), "ada'; drop table orders --", "12.50"}

	answered, err := build.BuildUndoUpdate(target, row, []int{1, 2})
	if err != nil {
		t.Fatalf("the undo answered %v", err)
	}
	if strings.Contains(answered.SQL, "drop table") {
		t.Errorf("the value was written into the statement:\n%s", answered.SQL)
	}
	if answered.SQL != `update "public"."orders" set "customer" = $1, "total" = $2 where "id" = $3` {
		t.Errorf("the undo is:\n%s", answered.SQL)
	}
	if len(answered.Params) != 3 {
		t.Fatalf("the undo binds %d values, wanted the two columns and the key",
			len(answered.Params))
	}
}

func TestBuildUndoUpdateComparesAKeyOfNothingWithIsNull(t *testing.T) {
	target := buildOrdersTarget("id")
	answered, err := build.BuildUndoUpdate(target, []any{nil, "ada", "1"}, []int{1})
	if err != nil {
		t.Fatalf("the undo answered %v", err)
	}
	if !strings.Contains(answered.SQL, `where "id" is null`) {
		t.Errorf("the undo is:\n%s", answered.SQL)
	}
}

func TestBuildShownUndoUpdateWritesTheValuesIn(t *testing.T) {
	target := buildOrdersTarget("id")
	written, err := build.BuildShownUndoUpdate(target, []any{int64(7), "ada", nil}, []int{1, 2})
	if err != nil {
		t.Fatalf("the undo answered %v", err)
	}
	if written != `update "public"."orders" set "customer" = 'ada', "total" = null where "id" = 7` {
		t.Errorf("the undo reads:\n%s", written)
	}
}

func TestBuildUndoInsertWritesEveryColumn(t *testing.T) {
	target := buildOrdersTarget("id")
	answered, err := build.BuildUndoInsert(target, []any{int64(7), "ada", "12.50"})
	if err != nil {
		t.Fatalf("the undo answered %v", err)
	}
	if answered.SQL !=
		`insert into "public"."orders" ("id", "customer", "total") values ($1, $2, $3)` {
		t.Errorf("the undo is:\n%s", answered.SQL)
	}
	if len(answered.Params) != 3 {
		t.Fatalf("the undo binds %d values, wanted every column", len(answered.Params))
	}
}

func TestBuildShownUndoInsertWritesTheValuesIn(t *testing.T) {
	target := buildOrdersTarget("id")
	written, err := build.BuildShownUndoInsert(target, []any{int64(7), nil, "12.50"})
	if err != nil {
		t.Fatalf("the undo answered %v", err)
	}
	if written !=
		`insert into "public"."orders" ("id", "customer", "total") values (7, null, '12.50')` {
		t.Errorf("the undo reads:\n%s", written)
	}
}

func TestBuildUndoUpdateRefusesARowWithoutItsKey(t *testing.T) {
	target := buildOrdersTarget("order_number")
	if _, err := build.BuildUndoUpdate(
		target, []any{int64(7), "ada", "1"}, []int{1}); err == nil {
		t.Error("a row that holds no key column was undone")
	}
}
