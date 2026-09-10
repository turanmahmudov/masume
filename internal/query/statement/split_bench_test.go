package statement_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// A buffer of many statements and a buffer that is one large block both split in one read of
// the tokens.
func BenchmarkSplitBigBuffer(b *testing.B) {
	sql := strings.Repeat("select id, name from orders where id = 1 order by id;\n", 20000)
	b.ReportAllocs()
	for b.Loop() {
		statement.SplitStatementRanges(sql, syntax.FlavourStandard)
	}
}

func BenchmarkSplitOneBigBlock(b *testing.B) {
	sql := "create procedure p() begin " +
		strings.Repeat("insert into t values (1); ", 20000) + " end;"
	b.ReportAllocs()
	for b.Loop() {
		statement.SplitStatementRanges(sql, syntax.FlavourStandard)
	}
}
