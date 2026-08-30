package statement

import (
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// A transaction the user typed. The screens hold a mark of what the connection is in, and
// a `begin` or a `commit` written into the editor has to move that mark, or the mark and
// the server drift apart: a staged write would then join a transaction that is already
// committed, and commit on its own.

// TransactionEffect is what a statement leaves the transaction of the user as.
type TransactionEffect string

// The three effects a statement can have.
const (
	// EffectNone leaves the transaction as it was.
	EffectNone TransactionEffect = "none"
	// EffectOpen opens one.
	EffectOpen TransactionEffect = "open"
	// EffectEnd ends the open one, whether it commits or rolls back.
	EffectEnd TransactionEffect = "end"
)

// transactionOpeners name the words that open a transaction. `begin` opens one in every
// dialect this client reads, and `start` only opens one before `transaction`.
var transactionOpeners = map[string]bool{"begin": true, "start": true}

// transactionEnders name the words that end one. `end` is a commit in PostgreSQL and in
// SQLite, and MySQL has no statement of that name.
var transactionEnders = map[string]bool{"commit": true, "rollback": true, "end": true}

// ResolveTransactionEffect returns what this buffer leaves the transaction as. The last
// statement that opens or ends one decides, because a buffer runs in order.
func ResolveTransactionEffect(sql string, flavour syntax.SyntaxFlavour) TransactionEffect {
	effect := EffectNone
	for _, one := range SplitStatements(sql, flavour) {
		if held := resolveStatementEffect(one, flavour); held != EffectNone {
			effect = held
		}
	}
	return effect
}

// resolveStatementEffect returns what one statement does to the transaction.
func resolveStatementEffect(sql string, flavour syntax.SyntaxFlavour) TransactionEffect {
	tokens := syntax.ReadCodeTokens(sql, flavour)
	opening := syntax.ReadOpeningWord(tokens)
	second, hasSecond := syntax.TokenAt(tokens, 1)

	if transactionOpeners[opening] {
		// `start` opens nothing on its own, and `begin` opens the body of a routine
		// where a word follows it that is not `transaction` or `work`.
		if opening == "start" {
			if hasSecond && second.Text == "transaction" {
				return EffectOpen
			}
			return EffectNone
		}
		if !hasSecond || second.Text == "transaction" || second.Text == "work" {
			return EffectOpen
		}
		return EffectNone
	}
	if transactionEnders[opening] {
		// `rollback to` returns to a savepoint and leaves the transaction open, and
		// `commit` or `rollback` with `chain` opens the next one straight away.
		if hasSecond && second.Text == "to" {
			return EffectNone
		}
		if holdsChainWord(tokens) {
			return EffectOpen
		}
		return EffectEnd
	}
	return EffectNone
}

// holdsChainWord is true for a `commit and chain`, which ends one transaction and opens
// the next in its place. `and no chain` is the plain end, and names the word too.
func holdsChainWord(tokens []syntax.CodeToken) bool {
	for at, token := range tokens {
		if token.Text != "chain" || !syntax.IsWordKind(token.Kind) {
			continue
		}
		before, present := syntax.TokenAt(tokens, at-1)
		return !present || before.Text != "no"
	}
	return false
}
