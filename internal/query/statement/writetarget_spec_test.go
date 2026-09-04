package statement_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

func TestReadWriteTargetReadsUpdate(t *testing.T) {
	target, read := statement.ReadWriteTarget(
		`update public.orders set status = 'x', total = 1 where id > 10 returning id`,
		syntax.FlavourStandard)
	if !read {
		t.Fatal("the update was not read")
	}
	if target.Kind != statement.WriteUpdate {
		t.Errorf("kind is %q", target.Kind)
	}
	if target.Table.Schema != "public" || target.Table.Name != "orders" {
		t.Errorf("relation is %q.%q", target.Table.Schema, target.Table.Name)
	}
	if strings.Join(target.Columns, ",") != "status,total" {
		t.Errorf("columns are %v", target.Columns)
	}
	if !target.HasWhere || target.Where != "id > 10" {
		t.Errorf("predicate is %q", target.Where)
	}
}

func TestReadWriteTargetKeepsCommasInsideCalls(t *testing.T) {
	target, read := statement.ReadWriteTarget(
		`update orders set label = concat(a, b), seen = now() where id = 1`,
		syntax.FlavourStandard)
	if !read {
		t.Fatal("the update was not read")
	}
	if strings.Join(target.Columns, ",") != "label,seen" {
		t.Errorf("columns are %v", target.Columns)
	}
}

func TestReadWriteTargetKeepsSubqueryPredicate(t *testing.T) {
	target, read := statement.ReadWriteTarget(
		`delete from orders where id in (select id from stale where kept = false)`,
		syntax.FlavourStandard)
	if !read {
		t.Fatal("the delete was not read")
	}
	if target.Kind != statement.WriteDelete {
		t.Errorf("kind is %q", target.Kind)
	}
	if target.Where != "id in (select id from stale where kept = false)" {
		t.Errorf("predicate is %q", target.Where)
	}
}

func TestReadWriteTargetReadsWriteOverEveryRow(t *testing.T) {
	for _, written := range []string{
		"delete from orders", "truncate table orders", "truncate orders",
	} {
		target, read := statement.ReadWriteTarget(written, syntax.FlavourStandard)
		if !read {
			t.Fatalf("%q was not read", written)
		}
		if target.HasWhere {
			t.Errorf("%q was read with a predicate", written)
		}
		if target.Table.Name != "orders" {
			t.Errorf("%q named %q", written, target.Table.Name)
		}
	}
}

func TestReadWriteTargetRefusesWhatItCannotCount(t *testing.T) {
	for _, written := range []string{
		`update orders o set status = 'x' where o.id = 1`,
		`update orders set status = s.name from states s where s.id = orders.state`,
		`delete from orders using states where states.id = orders.state`,
		`with kept as (select 1) delete from orders where id in (select * from kept)`,
		`truncate orders, order_lines`,
		`select * from orders`,
		`update orders set status = 'x' where id = 1; delete from orders`,
	} {
		if _, read := statement.ReadWriteTarget(written, syntax.FlavourStandard); read {
			t.Errorf("%q was read as a write of one relation", written)
		}
	}
}

func TestReadWriteTargetReadsInsert(t *testing.T) {
	target, read := statement.ReadWriteTarget(
		"insert into shop.orders (id) values (1)", syntax.FlavourStandard)
	if !read {
		t.Fatal("the insert was not read")
	}
	if target.Kind != statement.WriteInsert || target.Table.Name != "orders" {
		t.Errorf("read %q on %q", target.Kind, target.Table.Name)
	}
}

func TestReadWriteTargetRefusesMoreThanOneStatement(t *testing.T) {
	written := "update orders set status = 'x' where id = 1; truncate orders"
	if _, read := statement.ReadWriteTarget(written, syntax.FlavourStandard); read {
		t.Error("two statements were read as one write")
	}
}
