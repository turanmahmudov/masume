package present_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

func buildBigCatalog(schemas, perSchema int) present.TreeInput {
	tables := make([]db.TableRef, 0, schemas*perSchema)
	for s := range schemas {
		schema := fmt.Sprintf("schema_%02d", s)
		for at := range perSchema {
			tables = append(tables, db.TableRef{
				Schema: schema, Name: fmt.Sprintf("table_%04d", at),
				Kind: db.RelationTable, EstimatedRows: int64(at * 100),
			})
		}
	}
	return present.TreeInput{
		Engine: core.EnginePostgres, Tables: tables,
		Expanded: map[string]bool{},
		Details:  map[string]present.TableDetailState{},
		Now:      time.Now(),
	}
}

func BenchmarkBuildTreeFolded(b *testing.B) {
	for _, held := range []struct{ schemas, perSchema int }{
		{5, 20}, {20, 250}, {50, 500},
	} {
		input := buildBigCatalog(held.schemas, held.perSchema)
		b.Run(fmt.Sprintf("%dx%d=%d", held.schemas, held.perSchema,
			held.schemas*held.perSchema), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				present.BuildTree(input)
			}
		})
	}
}

func BenchmarkBuildTreeOneSchemaOpen(b *testing.B) {
	input := buildBigCatalog(20, 250)
	first := present.BuildTree(input)
	for _, row := range first.Rows {
		if row.Expandable {
			input.Expanded[row.ID] = true
			break
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		present.BuildTree(input)
	}
}

func BenchmarkBuildTreeFiltered(b *testing.B) {
	input := buildBigCatalog(20, 250)
	input.Filter = "table_0123"
	b.ReportAllocs()
	for b.Loop() {
		present.BuildTree(input)
	}
}
