package mongo

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

var ordersCollection = db.TableRef{Schema: "shop", Name: "orders", Kind: db.RelationTable}

// A collection keeps no schema, so the columns are what the documents hold. A field one
// document has and another has not still reads as one column.
func TestBuildDocumentResultAnswersTheFieldsOfEveryDocument(t *testing.T) {
	documents := []bson.D{
		{{Key: "_id", Value: int32(1)}, {Key: "total", Value: int32(5)}},
		{{Key: "_id", Value: int32(2)}, {Key: "note", Value: "late"}},
	}

	answered := BuildDocumentResult(documents, 0, "find")
	if len(answered.Columns) != 3 {
		t.Fatalf("the result holds %d columns, wanted three", len(answered.Columns))
	}
	if answered.Columns[0].Name != IdentityField {
		t.Errorf("the first column is %q, wanted the identity", answered.Columns[0].Name)
	}
	if len(answered.Rows) != 2 {
		t.Fatalf("the result holds %d rows, wanted two", len(answered.Rows))
	}
	// The second document has no total, and the cell of it is nothing rather than the
	// value of the row above.
	if answered.Rows[1][1] != nil {
		t.Errorf("the missing field reads %v, wanted nothing", answered.Rows[1][1])
	}
	if answered.Rows[1][2] != "late" {
		t.Errorf("the note reads %v", answered.Rows[1][2])
	}
}

// A field that holds one type reads as that type, and a field the sample saw under
// several is mixed, because no one type can be written back into it.
func TestBuildDocumentColumnsNamesTheTypeOfEachField(t *testing.T) {
	documents := []bson.D{
		{{Key: "total", Value: int32(5)}, {Key: "note", Value: "late"}},
		{{Key: "total", Value: "five"}, {Key: "note", Value: nil}},
	}

	columns := BuildDocumentColumns(documents)
	types := map[string]string{}
	for _, column := range columns {
		types[column.Name] = column.DataType
	}
	if types["total"] != TypeMixed {
		t.Errorf("the total reads as %q, wanted mixed", types["total"])
	}
	// A field that is null in one document is still of the type the other gave it.
	if types["note"] != TypeString {
		t.Errorf("the note reads as %q, wanted a string", types["note"])
	}
}

// A cell draws one line, so a document inside a field is written as its extended JSON
// and an identity as the text another client would print.
func TestFormatValueWritesEveryKindOfValueAsACell(t *testing.T) {
	identity, err := bson.ObjectIDFromHex("507f1f77bcf86cd799439011")
	if err != nil {
		t.Fatalf("the identity answered %v", err)
	}
	if held := FormatValue(identity); held != "507f1f77bcf86cd799439011" {
		t.Errorf("the identity reads %v", held)
	}
	if held := FormatValue(bson.D{{Key: "a", Value: int32(1)}}); held != `{"a":1}` {
		t.Errorf("the document reads %v", held)
	}
	if held := FormatValue(int32(5)); held != int64(5) {
		t.Errorf("the number reads %v", held)
	}
	if held := FormatValue(nil); held != nil {
		t.Errorf("nothing reads as %v", held)
	}
	moment := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	if held := FormatValue(bson.NewDateTimeFromTime(moment)); held != moment {
		t.Errorf("the date reads %v, wanted %v", held, moment)
	}
}

// The read of a relation names its database, so the statement reads the collection the
// tree named whichever database the connection opened.
func TestComposeRelationReadNamesTheDatabaseAndTheCollection(t *testing.T) {
	read := ComposeRelationRead(ordersCollection, core.ReadRewrite{})
	if read.Text != `db.getSiblingDB("shop").orders.find({})` {
		t.Errorf("the read is %q", read.Text)
	}
	if !read.Pageable {
		t.Error("a read of one collection reads as one that cannot be paged")
	}
}

// The sort and the filter of the tab go into the statement, so the server orders and
// narrows the read rather than the client narrowing the page it holds.
func TestComposeRelationReadWritesTheSortAndTheFilterIntoTheStatement(t *testing.T) {
	read := ComposeRelationRead(ordersCollection, core.ReadRewrite{
		Sort: []core.SortState{
			{Column: "total", Direction: core.SortDescending},
			{Column: "customer", Direction: core.SortAscending},
		},
		Filter: []core.FilterStep{
			{Kind: core.FilterCompare, Column: "status", Test: core.FilterEquals, Value: "new"},
			{Kind: core.FilterCompare, Column: "note", Test: core.FilterIsNull},
		},
	})

	if !strings.Contains(read.Text, `"$and"`) {
		t.Errorf("the two steps of the filter were not joined:\n%s", read.Text)
	}
	if !strings.Contains(read.Text, `{"status":"new"}`) {
		t.Errorf("the value of the filter was lost:\n%s", read.Text)
	}
	if !strings.Contains(read.Text, `{"note":null}`) {
		t.Errorf("the null step was lost:\n%s", read.Text)
	}
	if !strings.Contains(read.Text, `.sort({"total":-1,"customer":1})`) {
		t.Errorf("the sort was lost:\n%s", read.Text)
	}
	if _, err := ReadStatement(read.Text); err != nil {
		t.Errorf("the read is no statement this client can read: %v", err)
	}
}

// A value the user typed is written as a value, never as text the statement carries, or
// a quote in it would end the value early.
func TestComposeRelationReadWritesAValueAndNotTheTextOfOne(t *testing.T) {
	read := ComposeRelationRead(ordersCollection, core.ReadRewrite{
		Filter: []core.FilterStep{{
			Kind: core.FilterCompare, Column: "note",
			Test: core.FilterEquals, Value: `a"}) drop`,
		}},
	})
	parsed, err := ReadStatement(read.Text)
	if err != nil {
		t.Fatalf("the read is no statement this client can read: %v", err)
	}
	filter, readErr := ReadDocument(parsed.Calls[0].ReadArgument(0))
	if readErr != nil {
		t.Fatalf("the filter answered %v", readErr)
	}
	if len(filter) != 1 || filter[0].Value != `a"}) drop` {
		t.Errorf("the filter reads %v", filter)
	}
}

// An identity is read back as its hexadecimal text, so a filter over one has to name the
// identity itself or it would match nothing.
func TestComposeRelationReadWritesAnIdentityAsOne(t *testing.T) {
	read := ComposeRelationRead(ordersCollection, core.ReadRewrite{
		Filter: []core.FilterStep{{
			Kind: core.FilterCompare, Column: IdentityField,
			Test: core.FilterEquals, Value: "507f1f77bcf86cd799439011",
		}},
	})
	if !strings.Contains(read.Text, `"$oid"`) {
		t.Errorf("the identity was written as text:\n%s", read.Text)
	}
}

// buildTarget answers a staged target of two documents, as the grid holds one.
func buildTarget() db.ChangeTarget {
	return db.ChangeTarget{
		Table: ordersCollection,
		Columns: []db.ResultColumn{
			{Name: IdentityField, DataType: TypeObjectID},
			{Name: "total", DataType: TypeInt},
			{Name: "note", DataType: TypeString},
		},
		Rows: [][]any{
			{"507f1f77bcf86cd799439011", int64(5), "late"},
			{"507f1f77bcf86cd799439012", int64(7), nil},
		},
		KeyColumns: []string{IdentityField},
	}
}

// Every cell the grid staged in one row becomes one update, so a row of three edits is
// written once.
func TestBuildChangesWritesOneUpdatePerRow(t *testing.T) {
	staged := core.NewPendingChanges()
	staged.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 1, Value: core.CellValue{Kind: core.CellText, Text: "9"},
	}
	staged.Edits[core.BuildEditKey(0, 2)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 2, Value: core.CellValue{Kind: core.CellNull},
	}

	changes, err := BuildChanges(buildTarget(), staged)
	if err != nil {
		t.Fatalf("the staged work answered %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("the staged work becomes %d changes, wanted one", len(changes))
	}
	command, readErr := ReadWriteCommand(changes[0])
	if readErr != nil {
		t.Fatalf("the change answered %v", readErr)
	}
	if command.Kind != WriteUpdate {
		t.Errorf("the change is a %q", command.Kind)
	}
	// The type of the column decides how the text is read, and this one holds numbers.
	written := WriteExtendedJSON(command.Document)
	if !strings.Contains(written, `"total":9`) {
		t.Errorf("the total was not written as a number: %s", written)
	}
	if !strings.Contains(written, `"note":null`) {
		t.Errorf("the null was lost: %s", written)
	}
	if !strings.Contains(WriteExtendedJSON(command.Filter), `"$oid"`) {
		t.Errorf("the row was not named by its identity: %v", command.Filter)
	}
}

// A row marked for deletion is removed by its identity, and the cells staged in it are
// not written first.
func TestBuildChangesRemovesAMarkedRowAndSkipsItsEdits(t *testing.T) {
	staged := core.NewPendingChanges()
	staged.DeletedRows[1] = true
	staged.Edits[core.BuildEditKey(1, 1)] = core.CellEdit{
		RowIndex: 1, ColumnIndex: 1, Value: core.CellValue{Kind: core.CellText, Text: "9"},
	}

	changes, err := BuildChanges(buildTarget(), staged)
	if err != nil {
		t.Fatalf("the staged work answered %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("the staged work becomes %d changes, wanted one", len(changes))
	}
	command, _ := ReadWriteCommand(changes[0])
	if command.Kind != WriteDelete {
		t.Errorf("the change is a %q, wanted a delete", command.Kind)
	}
}

// A new document is written from the form, and each value keeps the type its column has.
func TestBuildChangesWritesANewDocument(t *testing.T) {
	staged := core.NewPendingChanges()
	staged.Inserts = append(staged.Inserts, map[string]any{
		"total": float64(3), "note": "new", "shipped": true,
	})

	changes, err := BuildChanges(buildTarget(), staged)
	if err != nil {
		t.Fatalf("the staged work answered %v", err)
	}
	command, _ := ReadWriteCommand(changes[0])
	if command.Kind != WriteInsert {
		t.Errorf("the change is a %q, wanted an insert", command.Kind)
	}
	written := WriteExtendedJSON(command.Document)
	if !strings.Contains(written, `"total":3`) || strings.Contains(written, `"total":3.0`) {
		t.Errorf("the total was not written as the whole number the column holds: %s", written)
	}
	// A field the sample never saw is still written, because a collection takes it.
	if !strings.Contains(written, `"shipped":true`) {
		t.Errorf("the field the sample never saw was dropped: %s", written)
	}
}

// The identity of a document is chosen by the server and never written, so an edit of
// it is refused rather than sent.
func TestBuildChangesRefusesAnEditOfTheIdentity(t *testing.T) {
	staged := core.NewPendingChanges()
	staged.Edits[core.BuildEditKey(0, 0)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 0, Value: core.CellValue{Kind: core.CellText, Text: "x"},
	}

	if _, err := BuildChanges(buildTarget(), staged); err == nil {
		t.Error("an edit of the identity was written as a change")
	}
}

// A read that answers no identity names no document, and the refusal says so rather
// than writing every document of the collection.
func TestBuildChangesRefusesARowItCannotName(t *testing.T) {
	target := buildTarget()
	target.Columns = target.Columns[1:]
	target.Rows = [][]any{{int64(5), "late"}}

	staged := core.NewPendingChanges()
	staged.DeletedRows[0] = true
	if _, err := BuildChanges(target, staged); err == nil {
		t.Error("a row with no identity was removed")
	}
}

// The text of a cell is read as the type of its column, because a collection has no
// schema for the server to check it against.
func TestBuildWriteValueReadsTheTextAsTheTypeOfItsColumn(t *testing.T) {
	for _, held := range []struct {
		dataType string
		written  string
		want     any
	}{
		{TypeString, "5", "5"},
		{TypeInt, "5", int32(5)},
		{TypeLong, "5", int64(5)},
		{TypeDouble, "5.5", 5.5},
		{TypeBool, "true", true},
	} {
		answered, err := BuildWriteValue(
			core.CellValue{Kind: core.CellText, Text: held.written}, held.dataType)
		if err != nil {
			t.Errorf("%q as a %s answered %v", held.written, held.dataType, err)
			continue
		}
		if answered != held.want {
			t.Errorf("%q as a %s reads %#v, wanted %#v",
				held.written, held.dataType, answered, held.want)
		}
	}
}

func TestBuildWriteValueRefusesTextThatIsNotTheTypeOfItsColumn(t *testing.T) {
	for _, held := range []struct{ dataType, written string }{
		{TypeInt, "five"},
		{TypeBool, "yes please"},
		{TypeObjectID, "not an identity"},
		{TypeDate, "one day"},
	} {
		if _, err := BuildWriteValue(
			core.CellValue{Kind: core.CellText, Text: held.written}, held.dataType); err == nil {
			t.Errorf("%q was read as a %s", held.written, held.dataType)
		}
	}
}

// A sort the user asked for in the grid goes into the statement, so the server orders
// the rows rather than the client ordering the page it happens to hold.
func TestComposeStatementReadLaysTheSortAndTheFilterOverAFind(t *testing.T) {
	read := ComposeStatementRead(
		db.BoundText{Text: `db.orders.find({status: "new"})`},
		core.ReadRewrite{
			Sort: []core.SortState{{Column: "total", Direction: core.SortDescending}},
			Filter: []core.FilterStep{{
				Kind: core.FilterCompare, Column: "customer",
				Test: core.FilterEquals, Value: "ada",
			}},
		})

	if !read.Pageable {
		t.Error("a find reads as a statement that cannot be paged")
	}
	if !strings.Contains(read.Text, `"$and"`) {
		t.Errorf("the filter of the tab was not joined to the one of the statement:\n%s", read.Text)
	}
	if !strings.Contains(read.Text, `.sort({"total":-1})`) {
		t.Errorf("the sort was lost:\n%s", read.Text)
	}
	if _, err := ReadStatement(read.Text); err != nil {
		t.Errorf("the read is no statement this client can read: %v", err)
	}
}

// A sort of the tab wins over the one the statement carries, because the user just
// asked for it.
func TestComposeStatementReadReplacesASortTheStatementCarries(t *testing.T) {
	read := ComposeStatementRead(
		db.BoundText{Text: `db.orders.find({}).sort({customer: 1})`},
		core.ReadRewrite{Sort: []core.SortState{{Column: "total", Direction: core.SortAscending}}})

	if strings.Contains(read.Text, "customer") {
		t.Errorf("the sort of the statement was kept beside the one of the tab:\n%s", read.Text)
	}
	if !strings.Contains(read.Text, `.sort({"total":1})`) {
		t.Errorf("the sort of the tab was lost:\n%s", read.Text)
	}
}

// A statement that says which documents it wants is left alone, and is not paged, or a
// page would change what it asked for.
func TestComposeStatementReadLeavesAStatementOfItsOwnAlone(t *testing.T) {
	for _, written := range []string{
		`db.orders.find({}).limit(10)`,
		`db.orders.aggregate([{$match: {}}])`,
		`db.runCommand({ping: 1})`,
		`not a statement`,
	} {
		read := ComposeStatementRead(db.BoundText{Text: written},
			core.ReadRewrite{Sort: []core.SortState{{Column: "total"}}})
		if read.Text != written {
			t.Errorf("%q was rewritten as %q", written, read.Text)
		}
		if read.Pageable {
			t.Errorf("%q reads as a statement that can be paged", written)
		}
	}
}

// A database of the user holds collections the server keeps for itself: the one that
// holds its views, the one that profiles it, and the buckets under a time series
// collection. None of them is something the user wrote, so the tree leaves them out.
func TestIsSystemCollectionNamesWhatTheServerKeepsForItself(t *testing.T) {
	for _, held := range []struct {
		name string
		want bool
	}{
		{"system.views", true},
		{"system.profile", true},
		{"system.js", true},
		{"system.buckets.weather", true},
		{"orders", false},
		{"customers", false},
		// A collection of the user may still carry the word, and it is not a prefix.
		{"systems", false},
	} {
		if answered := IsSystemCollection(held.name); answered != held.want {
			t.Errorf("%q reads as system=%v, wanted %v", held.name, answered, held.want)
		}
	}
}

// A row of a result is written back to the collection the statement read. Only a find
// reads one: an aggregation reshapes the documents, and a command answers something that
// is no document of a collection.
func TestFindStatementSourceNamesTheCollectionAFindReads(t *testing.T) {
	for _, held := range []struct {
		written string
		schema  string
		name    string
		want    bool
	}{
		{`db.orders.find({})`, "", "orders", true},
		{`db.orders.find({status: "new"}).sort({total: -1}).limit(5)`, "", "orders", true},
		{`db.orders.findOne({_id: 1})`, "", "orders", true},
		{`db.getSiblingDB("shop").orders.find({})`, "shop", "orders", true},
		{`db.getCollection("order lines").find({})`, "", "order lines", true},

		// These answer documents that are no document of one collection.
		{`db.orders.aggregate([{$group: {_id: "$status"}}])`, "", "", false},
		{`db.orders.distinct("status")`, "", "", false},
		{`db.orders.countDocuments({})`, "", "", false},
		{`db.runCommand({ping: 1})`, "", "", false},
		{`db.getCollectionNames()`, "", "", false},
		{`not a statement`, "", "", false},
	} {
		source, found := FindStatementSource(held.written)
		if found != held.want {
			t.Errorf("%q reads as editable=%v, wanted %v", held.written, found, held.want)
			continue
		}
		if !found {
			continue
		}
		if source.Name != held.name {
			t.Errorf("%q names collection %q, wanted %q", held.written, source.Name, held.name)
		}
		if source.Schema != held.schema || source.HasSchema != (held.schema != "") {
			t.Errorf("%q names database %q, wanted %q", held.written, source.Schema, held.schema)
		}
	}
}

// A transaction is a fact of the deployment, not of the engine. A replica set names
// itself, a router of a sharded cluster says what it is, and a standalone server answers
// neither.
func TestDeploymentHoldsTransactionsReadsWhatTheServerAnswered(t *testing.T) {
	for _, held := range []struct {
		name  string
		hello bson.D
		want  bool
	}{
		{"a replica set", bson.D{
			{Key: "setName", Value: "rs0"}, {Key: "isWritablePrimary", Value: true},
		}, true},
		{"a router of a sharded cluster", bson.D{
			{Key: "msg", Value: "isdbgrid"},
		}, true},
		{"a standalone server", bson.D{
			{Key: "isWritablePrimary", Value: true}, {Key: "maxWireVersion", Value: int32(27)},
		}, false},
		{"a reply that says nothing", bson.D{}, false},
		{"a name that is empty", bson.D{{Key: "setName", Value: ""}}, false},
	} {
		if answered := deploymentHoldsTransactions(held.hello); answered != held.want {
			t.Errorf("%s reads as holding a transaction=%v, wanted %v",
				held.name, answered, held.want)
		}
	}
}

// A BSON date is a moment to the millisecond, not a day. A column named `date` is drawn
// as a day alone, so the time of every document would be lost from the grid, the viewer
// and the file an export writes.
func TestADateColumnKeepsTheTimeOfDay(t *testing.T) {
	moment := time.Date(2025, 1, 12, 10, 30, 45, 0, time.UTC)
	held := FormatValue(bson.NewDateTimeFromTime(moment))

	written := core.FormatCell(held, TypeDate)
	if written != "2025-01-12 10:30:45.000" {
		t.Errorf("the moment reads %q, wanted the time of day with it", written)
	}
}
