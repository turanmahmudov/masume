package dump_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/clickhouse"
	"github.com/turanmahmudov/masume/internal/db/mysql"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/db/sqlite"
	"github.com/turanmahmudov/masume/internal/db/sqlserver"
	"github.com/turanmahmudov/masume/internal/dump"
	"github.com/turanmahmudov/masume/internal/query"
)

// fakeServer answers the reads of a dump from values held in the test.
type fakeServer struct {
	tables  []db.TableRef
	ddl     map[string][]string
	dialect *query.Dialect
	objects []db.SchemaObject
	// objectDDL is the definition of each object, by name.
	objectDDL     map[string][]string
	relationships []db.Relationship
	columns       []db.ResultColumn
	rows          [][]any
	// batches is the batch size of every stream call.
	batches []int
	readErr error
	readSQL []string
}

func (server *fakeServer) Describe() db.SessionDescriptor {
	return db.SessionDescriptor{
		Profile:       cfg.Profile{Engine: "postgres", Database: "shop"},
		ServerVersion: "16.2",
	}
}

func (server *fakeServer) Dialect() *query.Dialect {
	if server.dialect != nil {
		return server.dialect
	}
	return postgres.Dialect
}

func (server *fakeServer) ListTables(context.Context) ([]db.TableRef, error) {
	return server.tables, nil
}

func (server *fakeServer) ListSchemaObjects(context.Context) ([]db.SchemaObject, error) {
	return server.objects, nil
}

func (server *fakeServer) ListRelationships(context.Context) ([]db.Relationship, error) {
	return server.relationships, nil
}

func (server *fakeServer) BuildObjectDDL(
	_ context.Context, object db.SchemaObject,
) ([]string, error) {
	return server.objectDDL[object.Name], nil
}

func (server *fakeServer) BuildTableDDL(
	_ context.Context, table db.TableRef,
) ([]string, error) {
	return server.ddl[table.Name], nil
}

func (server *fakeServer) StreamQuery(
	_ context.Context, sql string, _ []any, batchSize int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	server.readSQL = append(server.readSQL, sql)
	server.batches = append(server.batches, batchSize)
	if server.readErr != nil {
		return 0, server.readErr
	}
	if err := onBatch(server.rows, server.columns); err != nil {
		return 0, err
	}
	return int64(len(server.rows)), nil
}

func buildServer() *fakeServer {
	return &fakeServer{
		tables: []db.TableRef{
			{Schema: "public", Name: "orders", Kind: db.RelationTable},
			{Schema: "public", Name: "order_view", Kind: db.RelationView},
			{Schema: "audit", Name: "log", Kind: db.RelationTable},
		},
		ddl: map[string][]string{
			"orders":     {"create table public.orders (", "    id integer", ");"},
			"order_view": {"create view public.order_view as select 1;"},
		},
		objects: []db.SchemaObject{
			{Schema: "public", Name: "order_seq", Kind: db.ObjectSequence},
			{Schema: "public", Name: "note_order", Kind: db.ObjectTrigger, Detail: "orders"},
			{Schema: "audit", Name: "other_seq", Kind: db.ObjectSequence},
		},
		objectDDL: map[string][]string{
			"order_seq":  {"create sequence public.order_seq;"},
			"note_order": {"create trigger note_order after update on public.orders;"},
		},
		columns: []db.ResultColumn{
			{Name: "id", DataType: "integer"}, {Name: "name", DataType: "text"},
		},
		rows: [][]any{{int64(1), "first"}, {int64(2), nil}},
	}
}

func TestWriteHoldsSchemaAndRows(t *testing.T) {
	server := buildServer()
	written := &strings.Builder{}
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentAll,
	}, written)
	if err != nil {
		t.Fatal(err)
	}
	if report.Tables != 1 || report.Rows != 2 || report.Views != 1 || report.Objects != 2 {
		t.Fatalf("report: %+v, want one table, two rows, one view and two objects", report)
	}

	text := written.String()
	for _, wanted := range []string{
		"-- masume dump",
		"-- server: postgres 16.2",
		"-- database: shop",
		"-- schema: public",
		"-- content: schema and rows",
		"-- tables: 1",
		"create table public.orders (",
		`insert into "public"."orders" ("id", "name") values (1, 'first');`,
	} {
		if !strings.Contains(text, wanted) {
			t.Errorf("the dump has no %q in it:\n%s", wanted, text)
		}
	}
	if strings.Contains(text, "audit") || strings.Contains(text, "other_seq") {
		t.Errorf("the dump holds a relation of another schema:\n%s", text)
	}
	// A definition follows everything it uses: the sequence, then the table, then the view
	// over it, then the trigger on it.
	wantedOrder := []string{
		"create sequence", "create table", "insert into", "create view", "create trigger",
	}
	at := 0
	for _, wanted := range wantedOrder {
		found := strings.Index(text[at:], wanted)
		if found < 0 {
			t.Fatalf("the dump holds no %q after the part before it:\n%s", wanted, text)
		}
		at += found
	}
	if len(server.batches) != 1 || server.batches[0] != dump.BatchRows {
		t.Errorf("batch sizes: %v, want one of %d", server.batches, dump.BatchRows)
	}
}

func TestWriteSchemaOnlyReadsNoRow(t *testing.T) {
	server := buildServer()
	written := &strings.Builder{}
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentSchema, DropsFirst: true,
	}, written)
	if err != nil {
		t.Fatal(err)
	}
	if report.Rows != 0 {
		t.Errorf("rows: %d, want 0", report.Rows)
	}
	if len(server.readSQL) != 0 {
		t.Errorf("the dump read %v, want no read", server.readSQL)
	}
	text := written.String()
	if !strings.Contains(text, `drop table if exists "public"."orders";`) {
		t.Errorf("the dump has no drop statement in it:\n%s", text)
	}
	if strings.Contains(text, "insert into") {
		t.Errorf("the dump holds rows:\n%s", text)
	}
}

func TestWriteRowsOnlyHoldsNoDefinition(t *testing.T) {
	server := buildServer()
	written := &strings.Builder{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentRows, DropsFirst: true,
	}, written); err != nil {
		t.Fatal(err)
	}
	text := written.String()
	if strings.Contains(text, "create table") || strings.Contains(text, "drop table") {
		t.Errorf("the dump holds a definition:\n%s", text)
	}
	if !strings.Contains(text, "insert into") {
		t.Errorf("the dump holds no row:\n%s", text)
	}
}

func TestWriteNamedTables(t *testing.T) {
	server := buildServer()
	written := &strings.Builder{}
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentRows,
		Tables: []db.TableRef{{Schema: "audit", Name: "log", Kind: db.RelationTable}},
	}, written)
	if err != nil {
		t.Fatal(err)
	}
	if report.Tables != 1 {
		t.Fatalf("tables: %d, want 1", report.Tables)
	}
	if len(server.readSQL) != 1 || server.readSQL[0] != `select * from "audit"."log"` {
		t.Errorf("reads: %v, want the named table alone", server.readSQL)
	}
}

func TestWriteEmptySchemaFails(t *testing.T) {
	server := buildServer()
	_, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "nothing", Content: dump.ContentAll,
	}, &strings.Builder{})
	if !errors.Is(err, dump.ErrNoTable) {
		t.Fatalf("error: %v, want %v", err, dump.ErrNoTable)
	}
}

func TestWriteReportsReadFailure(t *testing.T) {
	server := buildServer()
	server.readErr = errors.New("connection lost")
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentAll,
	}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "connection lost") {
		t.Fatalf("error: %v, want the read failure", err)
	}
	if report.Tables != 0 {
		t.Errorf("tables: %d, want 0", report.Tables)
	}
}

func TestFindContentNamed(t *testing.T) {
	content, known := dump.FindContentNamed(" Schema Only ")
	if !known || content != dump.ContentSchema {
		t.Fatalf("content: %q %v, want %q", content, known, dump.ContentSchema)
	}
	if _, known := dump.FindContentNamed("everything"); known {
		t.Error("an unknown name was taken")
	}
}

func TestWriteEmptyTableHoldsItsDefinition(t *testing.T) {
	server := buildServer()
	server.rows = nil
	written := &strings.Builder{}
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentAll,
	}, written)
	if err != nil {
		t.Fatal(err)
	}
	if report.Tables != 1 || report.Rows != 0 {
		t.Fatalf("report: %+v, want one table and no row", report)
	}
	text := written.String()
	if !strings.Contains(text, "create table public.orders (") {
		t.Errorf("the dump holds no definition:\n%s", text)
	}
	if strings.Contains(text, "insert into") {
		t.Errorf("the dump holds a row:\n%s", text)
	}
}

func TestWriteEndsADefinitionWithASemicolon(t *testing.T) {
	server := buildServer()
	// ClickHouse answers a definition without a terminator.
	server.ddl["orders"] = []string{"create table public.orders (id Int32) engine = Memory"}
	written := &strings.Builder{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentSchema,
	}, written); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(written.String(), "engine = Memory;") {
		t.Errorf("the definition has no terminator:\n%s", written.String())
	}
}

func TestSortByReferenceWritesAParentFirst(t *testing.T) {
	tables := []db.TableRef{
		{Schema: "public", Name: "lines"},
		{Schema: "public", Name: "orders"},
		{Schema: "public", Name: "customers"},
	}
	relationships := []db.Relationship{
		{
			Schema: "public", Table: "lines",
			ForeignKey: query.ForeignKey{TargetSchema: "public", TargetTable: "orders"},
		},
		{
			Schema: "public", Table: "orders",
			ForeignKey: query.ForeignKey{TargetSchema: "public", TargetTable: "customers"},
		},
	}

	sorted := dump.SortByReference(tables, relationships)
	names := make([]string, 0, len(sorted))
	for _, table := range sorted {
		names = append(names, table.Name)
	}
	if strings.Join(names, ",") != "customers,orders,lines" {
		t.Fatalf("order: %v, want the parent of each table before it", names)
	}
}

func TestSortByReferenceKeepsEveryTableOfACycle(t *testing.T) {
	tables := []db.TableRef{
		{Schema: "public", Name: "left"},
		{Schema: "public", Name: "right"},
		{Schema: "public", Name: "alone"},
	}
	relationships := []db.Relationship{
		{
			Schema: "public", Table: "left",
			ForeignKey: query.ForeignKey{TargetSchema: "public", TargetTable: "right"},
		},
		{
			Schema: "public", Table: "right",
			ForeignKey: query.ForeignKey{TargetSchema: "public", TargetTable: "left"},
		},
		// A table that names itself waits for nothing.
		{
			Schema: "public", Table: "alone",
			ForeignKey: query.ForeignKey{TargetSchema: "public", TargetTable: "alone"},
		},
	}

	sorted := dump.SortByReference(tables, relationships)
	if len(sorted) != 3 {
		t.Fatalf("tables: %d, want every table of the cycle", len(sorted))
	}
	seen := map[string]bool{}
	for _, table := range sorted {
		if seen[table.Name] {
			t.Fatalf("%s was written twice", table.Name)
		}
		seen[table.Name] = true
	}
}

// A dump written with --drop runs on a server that does not hold the object yet, so every
// DROP of every engine names `if exists`.
func TestDropStatementsNameIfExists(t *testing.T) {
	dialects := map[string]*query.Dialect{
		"postgres":   postgres.Dialect,
		"mysql":      mysql.Dialect,
		"sqlserver":  sqlserver.Dialect,
		"clickhouse": clickhouse.Dialect,
		"sqlite":     sqlite.Dialect,
	}
	objects := []db.SchemaObject{
		{Schema: "public", Name: "order_seq", Kind: db.ObjectSequence},
		{Schema: "public", Name: "order_kind", Kind: db.ObjectType},
		{Schema: "public", Name: "note_order", Kind: db.ObjectFunction},
		{Schema: "public", Name: "audit_order", Kind: db.ObjectTrigger, Detail: "orders"},
	}
	views := []db.TableRef{
		{Schema: "public", Name: "open_orders", Kind: db.RelationView},
		{Schema: "public", Name: "sales", Kind: db.RelationMaterializedView},
	}

	for name, dialect := range dialects {
		server := buildServer()
		server.dialect = dialect
		server.objects = objects
		server.tables = append(views, db.TableRef{
			Schema: "public", Name: "orders", Kind: db.RelationTable,
		})
		written := &strings.Builder{}
		if _, err := dump.Write(context.Background(), server, dump.Options{
			Schema: "public", Content: dump.ContentSchema, DropsFirst: true,
		}, written); err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		for line := range strings.SplitSeq(written.String(), "\n") {
			if !strings.HasPrefix(line, "drop ") {
				continue
			}
			if !strings.Contains(line, " if exists ") {
				t.Errorf("%s: %q names no `if exists`", name, line)
			}
		}
	}
}
