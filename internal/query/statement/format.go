package statement

import (
	"regexp"
	"strings"

	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// clauseStarts are the clauses that take their own line when a buffer is formatted.
var clauseStarts = []string{
	"select", "from", "where", "group by", "having", "order by", "limit", "offset",
	"union all", "union", "inner join", "left join", "right join", "full join",
	"cross join", "join",
}

// isSpaceKeeping is true for tokens with significant internal whitespace.
func isSpaceKeeping(kind syntax.TokenKind) bool {
	return kind == syntax.TokenString || kind == syntax.TokenQuoted || kind == syntax.TokenComment
}

var horizontalSpace = regexp.MustCompile(`[ \t]+`)

// collapseCodeWhitespace collapses horizontal whitespace outside strings, quoted identifiers, and comments.
func collapseCodeWhitespace(sql string, flavour syntax.SyntaxFlavour) string {
	var collapsed strings.Builder
	cursor := 0
	for _, token := range syntax.Tokenize(sql, flavour) {
		if !isSpaceKeeping(token.Kind) {
			continue
		}
		collapsed.WriteString(horizontalSpace.ReplaceAllString(sql[cursor:token.Start], " "))
		collapsed.WriteString(sql[token.Start:token.End])
		cursor = token.End
	}
	collapsed.WriteString(horizontalSpace.ReplaceAllString(sql[cursor:], " "))
	return collapsed.String()
}

// FormatStatement separates top-level clauses and collapses code whitespace, preserving literal and comment contents.
func FormatStatement(sql string, flavour syntax.SyntaxFlavour) string {
	hits := syntax.FindTopLevelKeywords(sql, clauseStarts, flavour)
	if len(hits) == 0 {
		return strings.TrimSpace(sql)
	}

	pieces := []string{}
	if hits[0].Start > 0 {
		pieces = append(pieces, strings.TrimSpace(sql[:hits[0].Start]))
	}
	for at, hit := range hits {
		end := len(sql)
		if at+1 < len(hits) {
			end = hits[at+1].Start
		}
		pieces = append(pieces, strings.TrimSpace(sql[hit.Start:end]))
	}

	written := make([]string, 0, len(pieces))
	for _, piece := range pieces {
		if piece == "" {
			continue
		}
		written = append(written, collapseCodeWhitespace(piece, flavour))
	}
	return strings.Join(written, "\n")
}
