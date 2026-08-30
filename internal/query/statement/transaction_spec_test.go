package statement_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// The client holds a mark of the transaction the connection is in. A `commit` written into
// the editor never reaches the method that ends one, so without this the mark would say
// open after the server had already committed, and a staged write would commit on its own.
func TestResolveTransactionEffectReadsWhatAStatementLeavesOpen(t *testing.T) {
	for _, held := range []struct {
		name    string
		sql     string
		flavour syntax.SyntaxFlavour
		want    statement.TransactionEffect
	}{
		{"a plain read", "select * from orders",
			syntax.FlavourStandard, statement.EffectNone},
		{"a write", "insert into orders (id) values (1)",
			syntax.FlavourStandard, statement.EffectNone},

		{"begin", "begin", syntax.FlavourStandard, statement.EffectOpen},
		{"begin transaction", "begin transaction",
			syntax.FlavourStandard, statement.EffectOpen},
		{"begin work", "begin work", syntax.FlavourStandard, statement.EffectOpen},
		{"start transaction", "start transaction",
			syntax.FlavourMysql, statement.EffectOpen},
		{"start on its own opens nothing", "start replica",
			syntax.FlavourMysql, statement.EffectNone},

		{"commit", "commit", syntax.FlavourStandard, statement.EffectEnd},
		{"rollback", "rollback", syntax.FlavourStandard, statement.EffectEnd},
		{"end", "end", syntax.FlavourStandard, statement.EffectEnd},
		{"commit and no chain", "commit and no chain",
			syntax.FlavourStandard, statement.EffectEnd},

		// A chain ends one transaction and opens the next in its place.
		{"commit and chain", "commit and chain",
			syntax.FlavourStandard, statement.EffectOpen},
		// A rollback to a savepoint leaves the transaction open.
		{"rollback to a savepoint", "rollback to savepoint one",
			syntax.FlavourStandard, statement.EffectNone},
		{"savepoint", "savepoint one", syntax.FlavourStandard, statement.EffectNone},
		{"release", "release savepoint one",
			syntax.FlavourStandard, statement.EffectNone},

		// A buffer runs in order, so the last statement that moves it decides.
		{"a buffer that opens and ends one",
			"begin; insert into orders (id) values (1); commit;",
			syntax.FlavourStandard, statement.EffectEnd},
		{"a buffer that opens one and leaves it open",
			"begin; insert into orders (id) values (1);",
			syntax.FlavourStandard, statement.EffectOpen},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered := statement.ResolveTransactionEffect(held.sql, held.flavour)
			if answered != held.want {
				t.Errorf("%q reads as %q, wanted %q", held.sql, answered, held.want)
			}
		})
	}
}
