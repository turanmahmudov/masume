package app

import "testing"

func TestBuildStatementLabel(t *testing.T) {
	cases := []struct {
		written string
		wanted  string
	}{
		{"select * from orders", "orders"},
		{"select o.* from public.orders o join customer c on c.id = o.customer_id", "orders"},
		{"select 1 as one", "select"},
		{"update orders set paid = true", "orders"},
		{"vacuum", "vacuum"},
		{"", "statement"},
		{"   ", "statement"},
		{"-- name: paid orders\nselect 1", "select"},
	}
	for _, held := range cases {
		if answered := BuildStatementLabel(held.written); answered != held.wanted {
			t.Errorf("%q is labelled %q, wanted %q", held.written, answered, held.wanted)
		}
	}
}

func TestStartKeepsTwoStatementsOfOneRelationApart(t *testing.T) {
	store := &ResultStore{}
	store.Start([]string{
		"select 1", "select 2", "select * from orders", "select * from orders", "select 3",
	}, 100)

	wanted := []string{"select", "select (2)", "orders", "orders (2)", "select (3)"}
	results := store.Results()
	if len(results) != len(wanted) {
		t.Fatalf("the store holds %d results, wanted %d", len(results), len(wanted))
	}
	for at, label := range wanted {
		if results[at].Label != label {
			t.Errorf("statement %d is labelled %q, wanted %q", at+1, results[at].Label, label)
		}
	}
}

func TestSelectResultStaysInsideTheBatch(t *testing.T) {
	store := &ResultStore{}
	store.Start([]string{"select 1", "select 2"}, 100)

	store.SelectResult(1)
	if store.ActiveIndex() != 1 {
		t.Errorf("the statement on screen is %d, wanted 1", store.ActiveIndex())
	}
	store.SelectResult(9)
	if store.ActiveIndex() != 1 {
		t.Errorf("a place past the batch moved the pane to %d", store.ActiveIndex())
	}
	store.SelectResult(-1)
	if store.ActiveIndex() != 0 {
		t.Errorf("a place before the batch moved the pane to %d", store.ActiveIndex())
	}
}
