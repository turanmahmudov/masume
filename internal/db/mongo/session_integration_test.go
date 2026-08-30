//go:build integration

// An integration test: it reads a real MongoDB. The server is started outside this code and
// named through MASUME_TEST_MONGO. Nothing here knows how it was started.
//
// MongoDB takes a call rather than SQL, and a collection keeps no schema, so the columns of a
// read are the fields the documents themselves hold.
package mongo_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/db/mongo"
)

// openOrders answers a session on a database holding three documents in one collection.
func openOrders(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.Mongo)
	dbtest.RunStatements(t, session,
		"db.orders.drop()",
		`db.orders.insertMany([
			{customer: "ada", total: 5, status: "new"},
			{customer: "grace", total: 7, status: "new"},
			{customer: "alan", total: 3, status: "sent"}
		])`,
	)
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(), "db.orders.drop()", dbtest.ReadEverything, nil)
	})
	return session
}

func TestServerRunsACallAndAnswersTheDocuments(t *testing.T) {
	session := openOrders(t)

	answered, err := session.RunQuery(context.Background(),
		`db.orders.find({status: "new"}).sort({total: 1})`, dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the call answered %v", err)
	}
	if len(answered.Rows) != 2 {
		t.Fatalf("the call gave %d rows, wanted the two that are new", len(answered.Rows))
	}
	// The columns are the fields of the documents, and every document has an identity.
	if answered.Columns[0].Name != mongo.IdentityField {
		t.Errorf("the first column is %q, wanted the identity", answered.Columns[0].Name)
	}
	if held := db.ReadAnyText(readCell(answered, 0, "customer")); held != "ada" {
		t.Errorf("the first customer reads %q, wanted the one with the lowest total", held)
	}
}

// The shell writes a document with bare keys and single quotes, and a user pastes one in
// as it stands.
func TestServerReadsTheDocumentTheShellWrites(t *testing.T) {
	session := openOrders(t)

	answered, err := session.RunQuery(context.Background(),
		`db.orders.countDocuments({total: {$gte: 5}})`, dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if held := db.ReadNonNegativeCount(answered.Rows[0][0]); held != 2 {
		t.Errorf("the count reads %d, wanted the 2 above five", held)
	}
}

// The tree of a MongoDB connection is its databases and the collections in each.
func TestServerListsTheCollectionsItHolds(t *testing.T) {
	session := openOrders(t)

	tables, err := session.ListTables(context.Background())
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}
	found := false
	for _, table := range tables {
		found = found || table.Name == "orders"
	}
	if !found {
		t.Errorf("the collections read %v, wanted orders among them", tables)
	}
}

// A collection keeps no schema, so the columns are read from a sample of its documents.
func TestServerDescribesACollectionFromItsDocuments(t *testing.T) {
	session := openOrders(t)

	detail, err := session.DescribeTable(context.Background(),
		db.TableRef{Schema: readDatabase(session), Name: "orders", Kind: db.RelationTable})
	if err != nil {
		t.Fatalf("the description answered %v", err)
	}
	types := map[string]string{}
	key := ""
	for _, column := range detail.Columns {
		types[column.Name] = column.DataType
		if column.IsPrimaryKey {
			key = column.Name
		}
	}
	if key != mongo.IdentityField {
		t.Errorf("the collection is identified by %q, wanted the identity", key)
	}
	if types["customer"] != mongo.TypeString {
		t.Errorf("the customer reads as %q, wanted a string", types["customer"])
	}
	if _, held := types["status"]; !held {
		t.Errorf("the fields read %v, wanted the status among them", types)
	}
}

// A page is a skip and a limit over the read, and the count is of the whole read rather
// than of the page.
func TestServerReadsOnePageOfACollection(t *testing.T) {
	session := openOrders(t)
	read := composeRead(session)

	page, err := session.ReadPage(context.Background(), read, db.ReadWindow{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("the page answered %v", err)
	}
	if len(page.Rows) != 2 {
		t.Fatalf("the page holds %d rows, wanted two", len(page.Rows))
	}
	if !page.Truncated {
		t.Error("a page with a row after it does not read as truncated")
	}

	counted, known, countErr := session.CountRead(context.Background(), read)
	if countErr != nil {
		t.Fatalf("the count answered %v", countErr)
	}
	if !known || counted != 3 {
		t.Errorf("the read counts %d rows, wanted the 3 written", counted)
	}
}

// A staged edit of the grid is applied by the identity of its row, and nothing else.
func TestServerAppliesTheStagedWorkOfTheGrid(t *testing.T) {
	session := openOrders(t)
	read := composeRead(session)
	page, err := session.ReadPage(context.Background(), read, db.ReadWindow{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("the page answered %v", err)
	}

	rowIndex := findRow(page, "customer", "ada")
	if rowIndex == -1 {
		t.Fatal("the row of ada was not read")
	}
	totalIndex := findColumn(page, "total")

	staged := core.NewPendingChanges()
	staged.Edits[core.BuildEditKey(rowIndex, totalIndex)] = core.CellEdit{
		RowIndex: rowIndex, ColumnIndex: totalIndex,
		Value: core.CellValue{Kind: core.CellText, Text: "11"},
	}
	changes, buildErr := mongo.BuildChanges(db.ChangeTarget{
		Table:   db.TableRef{Schema: readDatabase(session), Name: "orders"},
		Columns: page.Columns, Rows: page.Rows, KeyColumns: []string{mongo.IdentityField},
	}, staged)
	if buildErr != nil {
		t.Fatalf("the staged work answered %v", buildErr)
	}
	if applyErr := session.ApplyChanges(context.Background(), changes); applyErr != nil {
		t.Fatalf("the change answered %v", applyErr)
	}

	answered, readErr := session.RunQuery(context.Background(),
		`db.orders.find({customer: "ada"})`, dbtest.ReadEverything, nil)
	if readErr != nil {
		t.Fatalf("the read answered %v", readErr)
	}
	if held := db.ReadNonNegativeCount(readCell(answered, 0, "total")); held != 11 {
		t.Errorf("the total reads %d, wanted the 11 that was staged", held)
	}
}

// The server plans a read and measures it, which is what the plan pane draws.
func TestServerPlansAReadAndMeasuresIt(t *testing.T) {
	session := openOrders(t)

	plan, err := session.ExplainQuery(context.Background(),
		`db.orders.find({status: "new"})`, true)
	if err != nil {
		t.Fatalf("the plan answered %v", err)
	}
	if plan.Root.Label == "" {
		t.Error("the plan opens at a stage with no name")
	}
	if !plan.Analyzed {
		t.Error("a measured plan reads as one the server only worked out")
	}
	if plan.Raw == "" {
		t.Error("the plan carries none of the text the server answered")
	}
}

// A write is never planned, because the server would have to run it to plan it.
func TestServerRefusesToPlanAWrite(t *testing.T) {
	session := openOrders(t)

	if _, err := session.ExplainQuery(context.Background(),
		`db.orders.insertOne({total: 1})`, false); err == nil {
		t.Error("an insert was planned")
	}
}

// The port must say what this server does not do, rather than pretend.
func TestServerReportsWhatItCannotDo(t *testing.T) {
	session := dbtest.Open(t, dbtest.Mongo)

	held := session.Capabilities()
	if held.HasTransactions {
		t.Error("mongodb reports a transaction the client can drive")
	}
	if held.WritesDDL {
		t.Error("mongodb reports that the object menu can write SQL for it")
	}
	if err := session.BeginTransaction(context.Background()); err == nil {
		t.Error("a transaction opened on a server that holds none for this client")
	}
}

func TestServerAnswersACallItRefuses(t *testing.T) {
	session := openOrders(t)

	_, err := session.RunQuery(context.Background(),
		`db.orders.find({$notAnOperator: 1})`, dbtest.ReadEverything, nil)
	if err == nil {
		t.Fatal("a call the server refuses answered no error")
	}
	if described := db.DescribeError(err); described == "" {
		t.Error("the error is described as an empty text")
	}
}

// A statement this client cannot read is reported before anything is sent.
func TestServerReportsAStatementTheClientCannotRead(t *testing.T) {
	session := dbtest.Open(t, dbtest.Mongo)

	found, faulty := session.CheckStatement(context.Background(), "orders.find({})")
	if !faulty {
		t.Fatal("a statement that opens with no db was read as one")
	}
	if !strings.Contains(found.Message, "db") {
		t.Errorf("the fault reads %q", found.Message)
	}
}

// An export reads the whole collection a batch at a time, so it never holds all of it.
func TestServerStreamsACollectionForAnExport(t *testing.T) {
	session := openOrders(t)

	batches := 0
	total, err := session.StreamQuery(context.Background(),
		`db.orders.find({})`, nil, 2,
		func(rows [][]any, columns []db.ResultColumn) error {
			batches++
			if len(columns) == 0 {
				t.Error("a batch was written with no columns")
			}
			return nil
		})
	if err != nil {
		t.Fatalf("the export answered %v", err)
	}
	if total != 3 {
		t.Errorf("the export wrote %d rows, wanted the 3 written", total)
	}
	if batches != 2 {
		t.Errorf("the export wrote %d batches of two, wanted two", batches)
	}
}

// An index of a collection is what the tree shows under it, and the unique ones are the
// only promise MongoDB keeps about a field beside the identity.
func TestServerListsTheIndexesOfACollection(t *testing.T) {
	session := openOrders(t)
	dbtest.RunStatements(t, session,
		`db.orders.createIndex({customer: 1}, {unique: true, name: "customer_unique"})`)

	table := db.TableRef{Schema: readDatabase(session), Name: "orders", Kind: db.RelationTable}
	indexes, err := session.ListIndexes(context.Background(), table)
	if err != nil {
		t.Fatalf("the indexes answered %v", err)
	}
	unique, primary := false, false
	for _, index := range indexes {
		if index.Name == "customer_unique" && index.IsUnique {
			unique = true
		}
		primary = primary || index.IsPrimary
	}
	if !unique {
		t.Errorf("the indexes read %v, wanted the unique one among them", indexes)
	}
	if !primary {
		t.Error("the index over the identity is not marked as the primary one")
	}

	constraints, constraintErr := session.ListConstraints(context.Background(), table)
	if constraintErr != nil {
		t.Fatalf("the constraints answered %v", constraintErr)
	}
	if len(constraints) != 2 {
		t.Errorf("the collection promises %d things, wanted the identity and the unique one",
			len(constraints))
	}
}

// The definition of a collection is the calls that build it and its indexes again.
func TestServerWritesTheCallsThatBuildACollectionAgain(t *testing.T) {
	session := openOrders(t)
	dbtest.RunStatements(t, session, `db.orders.createIndex({total: -1})`)

	lines, err := session.BuildTableDDL(context.Background(),
		db.TableRef{Schema: readDatabase(session), Name: "orders", Kind: db.RelationTable})
	if err != nil {
		t.Fatalf("the definition answered %v", err)
	}
	written := strings.Join(lines, "\n")
	if !strings.Contains(written, `createCollection("orders")`) {
		t.Errorf("the definition does not create the collection:\n%s", written)
	}
	if !strings.Contains(written, "createIndex") || !strings.Contains(written, `"total":-1`) {
		t.Errorf("the definition does not build the index again:\n%s", written)
	}
	// Every collection is created with an index over its identity, so nothing rebuilds it.
	if strings.Contains(written, `"name":"_id_"`) {
		t.Errorf("the definition rebuilds the index of the identity:\n%s", written)
	}
}

// An aggregation reads only what the caller has room to draw. A pipeline answers with as
// many documents as it wants, and a cursor read to its end would hold every one of them in
// memory.
func TestServerReadsAnAggregationUpToTheRowLimit(t *testing.T) {
	session := openOrders(t)

	answered, err := session.RunQuery(context.Background(),
		`db.orders.aggregate([{$sort: {total: 1}}])`, 2, nil)
	if err != nil {
		t.Fatalf("the pipeline answered %v", err)
	}
	if len(answered.Rows) != 2 {
		t.Fatalf("the pipeline gave %d rows, wanted the two the limit allows", len(answered.Rows))
	}
	if !answered.Truncated {
		t.Error("a pipeline with a document after the limit does not read as truncated")
	}
}

// An aggregation answers documents of its own shape, which the grid draws like any other.
func TestServerRunsAnAggregation(t *testing.T) {
	session := openOrders(t)

	answered, err := session.RunQuery(context.Background(),
		`db.orders.aggregate([{$group: {_id: "$status", total: {$sum: "$total"}}}, {$sort: {_id: 1}}])`,
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the pipeline answered %v", err)
	}
	if len(answered.Rows) != 2 {
		t.Fatalf("the pipeline gave %d rows, wanted one per status", len(answered.Rows))
	}
	if held := db.ReadNonNegativeCount(readCell(answered, 0, "total")); held != 12 {
		t.Errorf("the total of the new orders reads %d, wanted 12", held)
	}
}

// The tree names how many documents each collection holds, which is read beside it.
func TestServerCountsTheDocumentsOfEachCollection(t *testing.T) {
	session := openOrders(t)

	tables, err := session.ListTables(context.Background())
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}
	for _, table := range tables {
		if table.Name == "orders" && table.Schema == readDatabase(session) {
			if table.EstimatedRows != 3 {
				t.Errorf("the collection holds %d documents, wanted the 3 written",
					table.EstimatedRows)
			}
			return
		}
	}
	t.Error("the orders collection was not listed")
}

// The activity list is what the server is running now, and the connection of this test is
// running the call that asks for it.
func TestServerListsWhatItIsRunning(t *testing.T) {
	session := dbtest.Open(t, dbtest.Mongo)

	activity, err := session.ListActivity(context.Background())
	if err != nil {
		t.Fatalf("the activity answered %v", err)
	}
	if len(activity) == 0 {
		t.Fatal("the server reports that it is running nothing at all")
	}
	for _, entry := range activity {
		if entry.PID != 0 {
			return
		}
	}
	t.Error("no running operation carries a number, so none of them could be ended")
}

// The users of a deployment live in the admin database, and a connection reads the ones
// of the database it opened.
func TestServerListsTheUsersItCanRead(t *testing.T) {
	session := dbtest.Open(t, dbtest.Mongo)

	if _, err := session.ListRoles(context.Background()); err != nil {
		t.Fatalf("the users answered %v", err)
	}
}

// Every call this client offers has to reach the server and answer something the grid
// can draw, or a user would meet a refusal from the client rather than from the server.
func TestServerRunsEveryCallThisClientOffers(t *testing.T) {
	session := openOrders(t)
	name := readDatabase(session)

	for _, held := range []struct {
		written  string
		affected int64
		rows     int
	}{
		{`db.orders.insertOne({customer: "edsger", total: 2, status: "new"})`, 1, 0},
		{`db.orders.insertMany([{customer: "kurt", total: 1}, {customer: "emmy", total: 4}])`, 2, 0},
		{`db.orders.updateMany({status: "new"}, {$set: {status: "sent"}})`, 3, 0},
		{`db.orders.replaceOne({customer: "kurt"}, {customer: "kurt", total: 6})`, 1, 0},
		{`db.orders.deleteMany({total: {$lt: 3}})`, 1, 0},
		{`db.orders.deleteOne({customer: "emmy"})`, 1, 0},
		{`db.orders.findOneAndUpdate({customer: "ada"}, {$set: {total: 8}})`, 0, 1},
		{`db.orders.estimatedDocumentCount()`, 0, 1},
		{`db.orders.distinct("status")`, 0, 1},
		{`db.orders.createIndex({customer: 1})`, 0, 1},
		{`db.orders.getIndexes()`, 0, 2},
		{`db.orders.dropIndex("customer_1")`, 0, 1},
		{`db.runCommand({ping: 1})`, 0, 1},
		{`db.adminCommand({ping: 1})`, 0, 1},
		{`db.getCollectionNames()`, 0, 1},
		{`db.` + `getSiblingDB("` + name + `").orders.countDocuments({})`, 0, 1},
	} {
		answered, err := session.RunQuery(
			context.Background(), held.written, dbtest.ReadEverything, nil)
		if err != nil {
			t.Errorf("%s answered %v", held.written, err)
			continue
		}
		if held.affected > 0 {
			if !answered.HasAffected || answered.Affected != held.affected {
				t.Errorf("%s changed %d documents, wanted %d",
					held.written, answered.Affected, held.affected)
			}
			continue
		}
		if len(answered.Rows) != held.rows {
			t.Errorf("%s gave %d rows, wanted %d", held.written, len(answered.Rows), held.rows)
		}
	}

	// A collection is created and removed by the calls of the shell, and the tree reads
	// the change on its next look.
	dbtest.RunStatements(t, session, `db.createCollection("spares")`, `db.spares.drop()`)
}

// A hosted deployment always has authentication turned on. The server answers a ping to
// anyone and refuses every other command, so a connection that carries no credentials
// looks open until the first read of it.
func TestServerWithAuthenticationOpensForTheUserItKnows(t *testing.T) {
	session := dbtest.Open(t, dbtest.MongoAuth)

	tables, err := session.ListTables(context.Background())
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}
	if len(tables) == 0 {
		t.Error("the connection read no collection at all")
	}
	if session.Describe().ServerVersion == "unknown" {
		t.Error("the version was not read, so the connection is not authenticated")
	}
}

// A profile that names no user is refused at the point the user can fix it, rather than
// opening and then failing on every read.
func TestServerWithAuthenticationRefusesAProfileWithNoUser(t *testing.T) {
	profile, _ := dbtest.BuildProfile(t, dbtest.MongoAuth)
	profile.User = ""

	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, "")
	if err == nil {
		_ = session.Close()
		t.Fatal("a connection with no credentials opened on a server that authenticates")
	}
	if described := db.DescribeError(err); !strings.Contains(described, "names none") {
		t.Errorf("the refusal reads %q, and does not say the profile names no user", described)
	}
}

// A password the server does not know is refused with what the server said.
func TestServerWithAuthenticationRefusesAPasswordItDoesNotKnow(t *testing.T) {
	profile, _ := dbtest.BuildProfile(t, dbtest.MongoAuth)

	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, "not the password")
	if err == nil {
		_ = session.Close()
		t.Fatal("a connection with the wrong password opened")
	}
	if !strings.Contains(strings.ToLower(db.DescribeError(err)), "authentication") {
		t.Errorf("the refusal reads %q", db.DescribeError(err))
	}
}

// An export runs the statement again. Only a find and an aggregation are read a batch at a
// time; every other statement answers one result, and the file has to hold what the grid
// showed rather than the documents behind it.
func TestServerExportsWhatEachStatementAnswered(t *testing.T) {
	session := openOrders(t)

	for _, held := range []struct {
		written string
		rows    int64
		first   string
	}{
		{`db.orders.find({})`, 3, "_id"},
		{`db.orders.aggregate([{$group: {_id: "$status", n: {$sum: 1}}}])`, 2, "_id"},
		// A count answers a count. Reading the documents instead would write three rows
		// into the file while the grid showed one.
		{`db.orders.countDocuments({})`, 1, "count"},
		{`db.orders.distinct("status")`, 2, "status"},
	} {
		total := int64(0)
		columns := []db.ResultColumn{}
		_, err := session.StreamQuery(context.Background(), held.written, nil, 2,
			func(rows [][]any, held []db.ResultColumn) error {
				total += int64(len(rows))
				columns = held
				return nil
			})
		if err != nil {
			t.Errorf("%s answered %v", held.written, err)
			continue
		}
		if total != held.rows {
			t.Errorf("%s wrote %d rows, wanted %d", held.written, total, held.rows)
		}
		if len(columns) == 0 || columns[0].Name != held.first {
			t.Errorf("%s wrote columns %v, wanted %q first", held.written, columns, held.first)
		}
	}
}

// An export runs the statement a second time, so one that writes would write again. It is
// refused rather than repeated.
func TestServerRefusesToExportAStatementThatWrites(t *testing.T) {
	session := openOrders(t)

	for _, written := range []string{
		`db.orders.insertOne({customer: "kurt"})`,
		`db.orders.deleteMany({})`,
		`db.orders.drop()`,
	} {
		total, err := session.StreamQuery(context.Background(), written, nil, 10,
			func([][]any, []db.ResultColumn) error { return nil })
		if err == nil {
			t.Errorf("%s was run again for an export", written)
		}
		if total != 0 {
			t.Errorf("%s wrote %d rows before it was refused", written, total)
		}
	}
	// Nothing ran: the three documents are still there.
	answered, err := session.RunQuery(context.Background(),
		`db.orders.countDocuments({})`, dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if held := db.ReadNonNegativeCount(answered.Rows[0][0]); held != 3 {
		t.Errorf("the collection holds %d documents, wanted the 3 written", held)
	}
}

// openReplicaSet answers a session on a deployment that holds a transaction, with three
// documents written outside one.
func openReplicaSet(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.MongoReplicaSet)
	dbtest.RunStatements(t, session,
		"db.orders.drop()",
		`db.orders.insertMany([{customer: "ada", total: 5}, {customer: "grace", total: 7},
			{customer: "alan", total: 3}])`)
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(), "db.orders.drop()",
			dbtest.ReadEverything, nil)
	})
	return session
}

// A transaction needs a replica set or a sharded cluster. A standalone server holds none,
// and the two report what they are rather than what the engine can do in general.
func TestServerReportsWhetherTheDeploymentHoldsATransaction(t *testing.T) {
	standalone := dbtest.Open(t, dbtest.Mongo)
	if standalone.Capabilities().HasTransactions {
		t.Error("a standalone server reports a transaction it does not hold")
	}
	if err := standalone.BeginTransaction(context.Background()); err == nil {
		t.Error("a transaction opened on a standalone server")
	}

	replicated := dbtest.Open(t, dbtest.MongoReplicaSet)
	if !replicated.Capabilities().HasTransactions {
		t.Error("a replica set reports that it holds no transaction")
	}
}

// Work done inside a transaction is not there until it commits, and is there afterwards.
func TestServerHoldsTheWorkOfATransactionUntilItCommits(t *testing.T) {
	session := openReplicaSet(t)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction answered %v", err)
	}
	if session.ReadTransactionState() != db.TransactionOpen {
		t.Fatalf("the state reads %q", session.ReadTransactionState())
	}
	dbtest.RunStatements(t, session, `db.orders.insertOne({customer: "emmy", total: 9})`)

	// The connection sees its own work.
	if held := countOrders(t, session); held != 4 {
		t.Errorf("the transaction reads %d documents, wanted the 4 it holds", held)
	}

	if err := session.CommitTransaction(ctx); err != nil {
		t.Fatalf("the commit answered %v", err)
	}
	if session.ReadTransactionState() != db.TransactionNone {
		t.Errorf("the state reads %q after a commit", session.ReadTransactionState())
	}
	if held := countOrders(t, session); held != 4 {
		t.Errorf("the commit left %d documents, wanted 4", held)
	}
}

// A rollback throws the work away, and the documents are as they were.
func TestServerThrowsAwayTheWorkOfATransactionThatRollsBack(t *testing.T) {
	session := openReplicaSet(t)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction answered %v", err)
	}
	dbtest.RunStatements(t, session,
		`db.orders.insertOne({customer: "emmy", total: 9})`,
		`db.orders.deleteOne({customer: "ada"})`)
	if held := countOrders(t, session); held != 3 {
		t.Errorf("the transaction reads %d documents", held)
	}

	if err := session.RollbackTransaction(ctx); err != nil {
		t.Fatalf("the rollback answered %v", err)
	}
	if held := countOrders(t, session); held != 3 {
		t.Errorf("the rollback left %d documents, wanted the 3 written first", held)
	}
	answered, err := session.RunQuery(ctx,
		`db.orders.countDocuments({customer: "ada"})`, dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if db.ReadNonNegativeCount(answered.Rows[0][0]) != 1 {
		t.Error("the document the transaction removed did not come back")
	}
}

// The server aborts a transaction the moment it refuses a call inside one, and answers
// every later call that it was aborted. The state has to say so, or the user meets the
// same refusal again instead of being told to roll back.
func TestServerMarksATransactionFailedWhenTheServerRefusesACall(t *testing.T) {
	session := openReplicaSet(t)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction answered %v", err)
	}
	if _, err := session.RunQuery(ctx,
		`db.orders.find({$nope: 1})`, dbtest.ReadEverything, nil); err == nil {
		t.Fatal("a filter the server refuses answered no error")
	}
	if held := session.ReadTransactionState(); held != db.TransactionFailed {
		t.Errorf("the state reads %q, wanted failed", held)
	}
	// A rollback is what clears it, and the transaction the server already aborted is
	// not reported as a second fault.
	if err := session.RollbackTransaction(ctx); err != nil {
		t.Errorf("the rollback of an aborted transaction answered %v", err)
	}
	if held := session.ReadTransactionState(); held != db.TransactionNone {
		t.Errorf("the state reads %q after a rollback", held)
	}
}

// A statement this client cannot read never reaches the server, so it cannot abort the
// transaction the user is holding.
func TestServerKeepsATransactionAStatementTheClientRefusedNeverReached(t *testing.T) {
	session := openReplicaSet(t)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction answered %v", err)
	}
	defer func() { _ = session.RollbackTransaction(ctx) }()

	if _, err := session.RunQuery(ctx, "orders.find({})", dbtest.ReadEverything, nil); err == nil {
		t.Fatal("a statement that opens with no db was read as one")
	}
	if held := session.ReadTransactionState(); held != db.TransactionOpen {
		t.Errorf("the state reads %q, wanted the transaction still open", held)
	}
	// The transaction still works.
	if held := countOrders(t, session); held != 3 {
		t.Errorf("the transaction reads %d documents", held)
	}
}

// The staged work of the grid joins the transaction of the user, so it is thrown away with
// everything else when the user rolls back.
func TestServerJoinsTheStagedWorkToTheTransactionOfTheUser(t *testing.T) {
	session := openReplicaSet(t)
	ctx := context.Background()
	read := mongo.ComposeRelationRead(
		db.TableRef{Schema: readDatabase(session), Name: "orders", Kind: db.RelationTable},
		core.ReadRewrite{})

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction answered %v", err)
	}
	page, pageErr := session.ReadPage(ctx, read, db.ReadWindow{Limit: 10, Offset: 0})
	if pageErr != nil {
		t.Fatalf("the page answered %v", pageErr)
	}
	staged := core.NewPendingChanges()
	staged.DeletedRows[0] = true
	changes, buildErr := mongo.BuildChanges(db.ChangeTarget{
		Table:   db.TableRef{Schema: readDatabase(session), Name: "orders"},
		Columns: page.Columns, Rows: page.Rows, KeyColumns: []string{mongo.IdentityField},
	}, staged)
	if buildErr != nil {
		t.Fatalf("the staged work answered %v", buildErr)
	}
	if applyErr := session.ApplyChanges(ctx, changes); applyErr != nil {
		t.Fatalf("the change answered %v", applyErr)
	}
	if held := countOrders(t, session); held != 2 {
		t.Errorf("the transaction reads %d documents after the delete", held)
	}

	if err := session.RollbackTransaction(ctx); err != nil {
		t.Fatalf("the rollback answered %v", err)
	}
	if held := countOrders(t, session); held != 3 {
		t.Errorf("the rollback left %d documents, wanted the 3 written first", held)
	}
}

// Without a transaction of the user, the staged work opens one of its own, so a set that
// fails part way through leaves nothing behind.
func TestServerAppliesStagedWorkAsOneOrNotAtAll(t *testing.T) {
	session := openReplicaSet(t)
	ctx := context.Background()
	dbtest.RunStatements(t, session,
		`db.orders.createIndex({customer: 1}, {unique: true, name: "customer_unique"})`)

	read := mongo.ComposeRelationRead(
		db.TableRef{Schema: readDatabase(session), Name: "orders", Kind: db.RelationTable},
		core.ReadRewrite{})
	page, pageErr := session.ReadPage(ctx, read, db.ReadWindow{Limit: 10, Offset: 0})
	if pageErr != nil {
		t.Fatalf("the page answered %v", pageErr)
	}

	// Two new documents that name the same customer. The index holds one of them only,
	// so the server refuses the second.
	staged := core.NewPendingChanges()
	staged.Inserts = append(staged.Inserts,
		map[string]any{"customer": "kurt", "total": float64(1)},
		map[string]any{"customer": "kurt", "total": float64(2)})
	changes, buildErr := mongo.BuildChanges(db.ChangeTarget{
		Table:   db.TableRef{Schema: readDatabase(session), Name: "orders"},
		Columns: page.Columns, Rows: page.Rows, KeyColumns: []string{mongo.IdentityField},
	}, staged)
	if buildErr != nil {
		t.Fatalf("the staged work answered %v", buildErr)
	}
	if len(changes) != 2 {
		t.Fatalf("the staged work becomes %d changes, wanted two", len(changes))
	}

	if err := session.ApplyChanges(ctx, changes); err == nil {
		t.Fatal("a set holding a change the server refuses was applied")
	}
	// The first insert was rolled back with the second, so neither landed.
	if held := countOrders(t, session); held != 3 {
		t.Errorf("the failed set left %d documents, wanted the 3 written first", held)
	}
	// The transaction it opened for itself is its own, and leaves the session free.
	if held := session.ReadTransactionState(); held != db.TransactionNone {
		t.Errorf("the session reads %q after a set of its own failed", held)
	}
}

func countOrders(t *testing.T, session db.Session) int64 {
	t.Helper()
	answered, err := session.RunQuery(context.Background(),
		"db.orders.countDocuments({})", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	return db.ReadNonNegativeCount(answered.Rows[0][0])
}

// A collection keeps no schema, and a file carries one header. A document read after the
// header is settled can hold a field the header has not, and a file that quietly drops it
// reads as a whole one. The export says which fields it could not write.
func TestServerReportsTheFieldsAnExportCouldNotWrite(t *testing.T) {
	session := openOrders(t)
	dbtest.RunStatements(t, session,
		`db.orders.insertOne({customer: "kurt", total: 1, refunded: true, note: "late"})`)

	total := int64(0)
	// One document to a batch, so the header is settled before the last one is read.
	_, err := session.StreamQuery(context.Background(), `db.orders.find({})`, nil, 1,
		func(rows [][]any, _ []db.ResultColumn) error {
			total += int64(len(rows))
			return nil
		})
	if err == nil {
		t.Fatal("the export dropped two fields and reported nothing")
	}
	described := db.DescribeError(err)
	for _, wanted := range []string{"refunded", "note", "was written"} {
		if !strings.Contains(described, wanted) {
			t.Errorf("the message does not name %q: %s", wanted, described)
		}
	}
	// Every row was still written: the file holds the rows, not fewer.
	if total != 4 {
		t.Errorf("the export wrote %d rows, wanted all 4", total)
	}
}

// A collection whose documents hold the same fields reports nothing, because nothing was
// left out.
func TestServerReportsNothingWhereEveryFieldWasWritten(t *testing.T) {
	session := openOrders(t)

	if _, err := session.StreamQuery(context.Background(), `db.orders.find({})`, nil, 1,
		func([][]any, []db.ResultColumn) error { return nil }); err != nil {
		t.Errorf("an export that wrote every field answered %v", err)
	}
}

// composeRead answers the read of the orders collection, as a table tab composes one.
func composeRead(session db.Session) db.ComposedRead {
	return mongo.ComposeRelationRead(
		db.TableRef{Schema: readDatabase(session), Name: "orders", Kind: db.RelationTable},
		core.ReadRewrite{})
}

func readDatabase(session db.Session) string {
	return session.Describe().DefaultSchema
}

func findColumn(result db.QueryResult, name string) int {
	for at, column := range result.Columns {
		if column.Name == name {
			return at
		}
	}
	return -1
}

func findRow(result db.QueryResult, column, value string) int {
	at := findColumn(result, column)
	if at == -1 {
		return -1
	}
	for rowIndex, row := range result.Rows {
		if db.ReadAnyText(row[at]) == value {
			return rowIndex
		}
	}
	return -1
}

func readCell(result db.QueryResult, rowIndex int, column string) any {
	at := findColumn(result, column)
	if at == -1 || rowIndex >= len(result.Rows) {
		return nil
	}
	return result.Rows[rowIndex][at]
}

// The old shell removes one document where `remove` is given a justOne. Sent as a
// deleteMany, a remove of one would empty the collection instead.
func TestRemoveWithJustOneRemovesOneDocument(t *testing.T) {
	session := openOrders(t)
	ctx := context.Background()

	dbtest.RunStatements(t, session, `db.probe.insertMany([{n: 1}, {n: 2}, {n: 3}])`)
	t.Cleanup(func() {
		_, _ = session.RunQuery(ctx, `db.probe.drop()`, dbtest.ReadEverything, nil)
	})

	if _, err := session.RunQuery(
		ctx, `db.probe.remove({}, true)`, dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the remove failed: %v", err)
	}
	if held := countProbeDocuments(t, session); held != 2 {
		t.Errorf("%d documents are left after a remove of one, wanted 2", held)
	}

	// A remove that names no justOne still removes every match.
	if _, err := session.RunQuery(
		ctx, `db.probe.remove({})`, dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the second remove failed: %v", err)
	}
	if held := countProbeDocuments(t, session); held != 0 {
		t.Errorf("%d documents are left after a remove of every match, wanted none", held)
	}
}

// countProbeDocuments answers how many documents the probe collection holds.
func countProbeDocuments(t *testing.T, session db.Session) int64 {
	t.Helper()
	held, err := session.RunQuery(
		context.Background(), `db.probe.countDocuments({})`, dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the count failed: %v", err)
	}
	if len(held.Rows) == 0 || len(held.Rows[0]) == 0 {
		t.Fatalf("the count answered no rows")
	}
	return db.ReadNonNegativeCount(held.Rows[0][0])
}
