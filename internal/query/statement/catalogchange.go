package statement

import (
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// shapingOpeners is the set of opening keywords that change schema objects.
var shapingOpeners = map[string]bool{
	"create": true, "drop": true, "alter": true, "rename": true, "comment": true,
}

// ChangesCatalog detects statement forms that require catalog refresh.
func ChangesCatalog(sql string, flavour syntax.SyntaxFlavour) bool {
	tokens := syntax.ReadCodeTokens(sql, flavour)
	if syntax.ReadOpeningWord(tokens) == "" {
		return false
	}
	return shapingOpeners[syntax.ReadOpeningWord(tokens)] || syntax.SelectsIntoTarget(tokens)
}
