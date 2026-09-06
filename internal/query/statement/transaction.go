package statement

import (
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Transaction state classification for statements entered in the editor.

// TransactionEffect is a statement effect on transaction state.
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

// transactionOpeners is the set of possible transaction opening keywords. START requires TRANSACTION.
var transactionOpeners = map[string]bool{"begin": true, "start": true}

// transactionEnders is the set of transaction closing keywords. PostgreSQL and SQLite accept END as COMMIT.
var transactionEnders = map[string]bool{"commit": true, "rollback": true, "end": true}

// ResolveTransactionEffect returns the last nonempty transaction effect in statement order.
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
		// Recognize START TRANSACTION and BEGIN with optional TRANSACTION or WORK.
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
		// ROLLBACK TO keeps the transaction open. CHAIN starts a new transaction.
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

// holdsChainWord detects CHAIN without a preceding NO.
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
