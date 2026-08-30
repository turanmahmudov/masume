package statement

import (
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// shapingOpeners are the words a statement starts with to create, drop or change
// an object.
var shapingOpeners = map[string]bool{
	"create": true, "drop": true, "alter": true, "rename": true, "comment": true,
}

// ChangesCatalog is true if the object tree and the name checker are stale after
// this statement. A write of rows changes no objects, so it is left out.
func ChangesCatalog(sql string, flavour syntax.SyntaxFlavour) bool {
	tokens := syntax.ReadCodeTokens(sql, flavour)
	if syntax.ReadOpeningWord(tokens) == "" {
		return false
	}
	return shapingOpeners[syntax.ReadOpeningWord(tokens)] || syntax.SelectsIntoTarget(tokens)
}
