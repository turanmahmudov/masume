// Package mongo opens a MongoDB server: the statements of the shell it reads, the
// documents it returns as rows, and the commands a staged edit becomes.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// applicationName is the name this client reports to the server, which the activity
// list of another client shows.
const applicationName = "masume"

// defaultDatabase is the database a profile that names none reads, which is the one the
// shell opens.
const defaultDatabase = "test"

// countedCollections is how many collections the catalog counts the documents of. A
// deployment with more than this returns its tree without the counts, because each one
// is a call of its own.
const countedCollections = 200

// countWorkers is how many counts the catalog asks for at a time.
const countWorkers = 8

// connectTimeout is how long the driver waits for a server to answer.
const connectTimeout = 10 * time.Second

// mongoSession is one session on a MongoDB server.
type mongoSession struct {
	db.PlainCatalog
	db.NoServerLoad
	db.SessionFacts

	client *mongo.Client
	// holdsTransactions is what the deployment answered when the connection opened. A
	// replica set and a sharded cluster hold a transaction; a standalone server has none.
	holdsTransactions bool
	transaction       *transactionHolder
}

// Capabilities returns what this connection does. Every entry is a fact of the engine
// except the transaction, which is a fact of the deployment the connection reached.
func (session *mongoSession) Capabilities() core.Capabilities {
	held := session.Support.Capabilities
	held.HasTransactions = session.holdsTransactions
	// A standalone server applies a staged set one change at a time, so a failure part
	// way through leaves the changes before it written.
	held.AppliesChangesTogether = session.holdsTransactions
	return held
}

// readDatabase returns the database of a statement, which is the one of the connection
// where the statement names none.
func (session *mongoSession) readDatabase(named string) *mongo.Database {
	if named == "" {
		return session.client.Database(session.Descriptor.DefaultSchema)
	}
	return session.client.Database(named)
}

// readCollection returns the collection a statement names.
func (session *mongoSession) readCollection(parsed Statement) *mongo.Collection {
	return session.readDatabase(parsed.Database).Collection(parsed.Collection)
}

// Ping returns whether the server is still there.
func (session *mongoSession) Ping(ctx context.Context) error {
	return session.client.Ping(ctx, readpref.Primary())
}

// Close ends the connection, and the transaction of the user with it. A transaction that
// was never committed is thrown away by the server.
func (session *mongoSession) Close() error {
	if opened := session.transaction.takeOpened(); opened != nil {
		opened.EndSession(context.Background())
		session.transaction.held.WriteState(db.TransactionNone)
	}
	return session.client.Disconnect(context.Background())
}

// RunQuery runs the statements of a buffer in order, and returns the result of the last
// one.
func (session *mongoSession) RunQuery(
	ctx context.Context, buffer string, rowLimit int, _ []any,
) (db.QueryResult, error) {
	startedAt := time.Now()
	written := session.Support.Language.SplitStatements(buffer)
	if len(written) == 0 {
		return db.QueryResult{Elapsed: time.Since(startedAt)}, nil
	}

	// Every statement is read before any of them runs, so a statement this client
	// cannot read never reaches a transaction and never aborts one.
	parsed := make([]Statement, 0, len(written))
	for _, one := range written {
		held, err := ReadStatement(one)
		if err != nil {
			return db.QueryResult{}, db.WrapDatabaseError(err)
		}
		parsed = append(parsed, held)
	}

	bound, giveBack, holdErr := session.holdSession(ctx)
	if holdErr != nil {
		return db.QueryResult{}, holdErr
	}
	defer giveBack()

	answered := db.QueryResult{}
	for _, one := range parsed {
		held, runErr := session.runStatement(bound, one, rowLimit)
		if runErr != nil {
			return db.QueryResult{}, session.noteServerFailure(runErr)
		}
		answered = held
	}
	answered.Elapsed = time.Since(startedAt)
	return answered, nil
}

// runStatement runs one statement, by the call it makes.
func (session *mongoSession) runStatement(
	ctx context.Context, parsed Statement, rowLimit int,
) (db.QueryResult, error) {
	if parsed.Collection == "" {
		return session.runDatabaseCall(ctx, parsed)
	}
	return session.runCollectionCall(ctx, parsed, rowLimit)
}

// runDatabaseCall runs a call made on the database itself.
func (session *mongoSession) runDatabaseCall(
	ctx context.Context, parsed Statement,
) (db.QueryResult, error) {
	call := parsed.Calls[0]
	database := session.readDatabase(parsed.Database)

	switch call.Name {
	case "runCommand", "adminCommand":
		command, err := ReadDocument(call.ReadArgument(0))
		if err != nil {
			return db.QueryResult{}, db.WrapDatabaseError(err)
		}
		if call.Name == "adminCommand" {
			database = session.client.Database("admin")
		}
		return session.readCommandReply(ctx, database, command, call.Name)
	case "getCollectionNames":
		names, err := database.ListCollectionNames(ctx, bson.D{})
		if err != nil {
			return db.QueryResult{}, db.WrapDatabaseError(err)
		}
		sort.Strings(names)
		rows := make([][]any, 0, len(names))
		for _, name := range names {
			rows = append(rows, []any{name})
		}
		return db.QueryResult{
			Columns: []db.ResultColumn{{Name: "collection", DataType: TypeString}},
			Rows:    rows, Command: call.Name,
		}, nil
	case "createCollection":
		name, err := ReadText(call.ReadArgument(0))
		if err != nil {
			return db.QueryResult{}, db.WrapDatabaseError(err)
		}
		if createErr := database.CreateCollection(ctx, name); createErr != nil {
			return db.QueryResult{}, db.WrapDatabaseError(createErr)
		}
		return BuildValueResult("created", name, 0, call.Name), nil
	case "dropDatabase":
		if err := database.Drop(ctx); err != nil {
			return db.QueryResult{}, db.WrapDatabaseError(err)
		}
		return BuildValueResult("dropped", database.Name(), 0, call.Name), nil
	}
	return db.QueryResult{}, db.NewDatabaseError(
		"this client does not run %s on a database", call.Name)
}

// readCommandReply runs one command and returns its reply as a result.
func (session *mongoSession) readCommandReply(
	ctx context.Context, database *mongo.Database, command bson.D, name string,
) (db.QueryResult, error) {
	var reply bson.D
	if err := database.RunCommand(ctx, command).Decode(&reply); err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	return BuildDocumentResult([]bson.D{reply}, 0, name), nil
}

// runCollectionCall runs a call made on one collection.
func (session *mongoSession) runCollectionCall(
	ctx context.Context, parsed Statement, rowLimit int,
) (db.QueryResult, error) {
	call := parsed.Calls[0]
	collection := session.readCollection(parsed)

	switch call.Name {
	case "find", "findOne":
		return session.readFind(ctx, parsed, rowLimit)
	case "aggregate":
		return session.readAggregate(ctx, parsed, rowLimit)
	case "countDocuments", "count":
		return session.readCount(ctx, parsed)
	case "estimatedDocumentCount":
		counted, err := collection.EstimatedDocumentCount(ctx)
		if err != nil {
			return db.QueryResult{}, db.WrapDatabaseError(err)
		}
		return BuildValueResult("count", counted, 0, call.Name), nil
	case "distinct":
		return session.readDistinct(ctx, parsed, rowLimit)
	case "getIndexes":
		return session.readIndexDocuments(ctx, parsed)
	case "insertOne", "insertMany", "insert":
		return session.writeInsert(ctx, parsed)
	case "updateOne", "updateMany", "replaceOne":
		return session.writeUpdate(ctx, parsed)
	case "deleteOne", "deleteMany", "remove":
		return session.writeDelete(ctx, parsed)
	case "findOneAndUpdate", "findOneAndReplace", "findOneAndDelete":
		return session.writeFindAnd(ctx, parsed)
	case "createIndex":
		return session.writeCreateIndex(ctx, parsed)
	case "dropIndex":
		name, err := ReadText(call.ReadArgument(0))
		if err != nil {
			return db.QueryResult{}, db.WrapDatabaseError(err)
		}
		if dropErr := collection.Indexes().DropOne(ctx, name); dropErr != nil {
			return db.QueryResult{}, db.WrapDatabaseError(dropErr)
		}
		return BuildValueResult("dropped", name, 0, call.Name), nil
	case "drop":
		if err := collection.Drop(ctx); err != nil {
			return db.QueryResult{}, db.WrapDatabaseError(err)
		}
		return BuildValueResult("dropped", parsed.Collection, 0, call.Name), nil
	}
	return db.QueryResult{}, db.NewDatabaseError(
		"this client does not run %s on a collection", call.Name)
}

// findPlan is a find with everything the chained calls added to it.
type findPlan struct {
	filter     bson.D
	projection bson.D
	sort       bson.D
	limit      int64
	hasLimit   bool
	skip       int64
}

// buildFindPlan reads a find and the calls chained onto it.
func buildFindPlan(parsed Statement) (findPlan, error) {
	call := parsed.Calls[0]
	plan := findPlan{}

	filter, err := ReadDocument(call.ReadArgument(0))
	if err != nil {
		return plan, err
	}
	plan.filter = filter

	if written := call.ReadArgument(1); written != "" {
		projection, projectionErr := ReadDocument(written)
		if projectionErr != nil {
			return plan, projectionErr
		}
		plan.projection = projection
	}
	if call.Name == "findOne" {
		plan.limit, plan.hasLimit = 1, true
	}

	for _, chained := range parsed.Calls[1:] {
		if err := plan.take(chained); err != nil {
			return plan, err
		}
	}
	return plan, nil
}

// take reads one chained call into the plan.
func (plan *findPlan) take(call MethodCall) error {
	switch call.Name {
	case "sort":
		held, err := ReadDocument(call.ReadArgument(0))
		if err != nil {
			return err
		}
		plan.sort = held
	case "projection":
		held, err := ReadDocument(call.ReadArgument(0))
		if err != nil {
			return err
		}
		plan.projection = held
	case "limit":
		held, err := readNumber(call.ReadArgument(0))
		if err != nil {
			return err
		}
		plan.limit, plan.hasLimit = held, true
	case "skip":
		held, err := readNumber(call.ReadArgument(0))
		if err != nil {
			return err
		}
		plan.skip = held
	case "pretty", "toArray", "batchSize", "hint", "allowDiskUse", "collation":
		// These change how a reply is printed or read, and the grid prints it itself.
	default:
		return newSyntaxError(call.Name + " is not a call this client chains onto a find")
	}
	return nil
}

// readNumber reads an argument that is a whole number.
func readNumber(written string) (int64, error) {
	value, err := ReadValue(written)
	if err != nil {
		return 0, err
	}
	switch held := value.(type) {
	case int32:
		return int64(held), nil
	case int64:
		return held, nil
	case float64:
		return int64(held), nil
	}
	return 0, newSyntaxError("this argument is a whole number")
}

// buildFindOptions returns the options the plan asks the server for, capped at the rows
// the caller wants.
func (plan findPlan) buildFindOptions(rowLimit int) *options.FindOptionsBuilder {
	held := options.Find()
	if plan.sort != nil {
		held.SetSort(plan.sort)
	}
	if plan.projection != nil {
		held.SetProjection(plan.projection)
	}
	if plan.skip > 0 {
		held.SetSkip(plan.skip)
	}
	if limit, capped := plan.resolveLimit(rowLimit); capped {
		held.SetLimit(limit)
	}
	return held
}

// resolveLimit returns the number of documents to ask for: the lower of what the
// statement asked for and what the caller has room to draw.
func (plan findPlan) resolveLimit(rowLimit int) (int64, bool) {
	wanted := int64(0)
	if rowLimit > 0 {
		wanted = int64(db.ReadOverscanRowLimit(rowLimit))
	}
	switch {
	case plan.hasLimit && wanted > 0:
		return min(plan.limit, wanted), true
	case plan.hasLimit:
		return plan.limit, true
	case wanted > 0:
		return wanted, true
	}
	return 0, false
}

// readFind runs a find and returns the documents it read.
func (session *mongoSession) readFind(
	ctx context.Context, parsed Statement, rowLimit int,
) (db.QueryResult, error) {
	plan, err := buildFindPlan(parsed)
	if err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	documents, readErr := session.readCursor(ctx, func() (*mongo.Cursor, error) {
		return session.readCollection(parsed).Find(
			ctx, plan.filter, plan.buildFindOptions(rowLimit))
	})
	if readErr != nil {
		return db.QueryResult{}, readErr
	}

	answered := BuildDocumentResult(documents, 0, parsed.ReadMethod())
	return db.BuildCappedResult(db.CappedRead{
		Rows: answered.Rows, RowLimit: rowLimit, Columns: answered.Columns,
		Command: parsed.ReadMethod(),
	}), nil
}

// readAggregate runs a pipeline and returns the documents it read.
func (session *mongoSession) readAggregate(
	ctx context.Context, parsed Statement, rowLimit int,
) (db.QueryResult, error) {
	pipeline, err := ReadArray(parsed.Calls[0].ReadArgument(0))
	if err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	documents, readErr := session.readCursorUpTo(ctx, rowLimit, func() (*mongo.Cursor, error) {
		return session.readCollection(parsed).Aggregate(ctx, pipeline)
	})
	if readErr != nil {
		return db.QueryResult{}, readErr
	}

	answered := BuildDocumentResult(documents, 0, "aggregate")
	return db.BuildCappedResult(db.CappedRead{
		Rows: answered.Rows, RowLimit: rowLimit, Columns: answered.Columns,
		Command: "aggregate",
	}), nil
}

// readCount counts the documents a filter matches.
func (session *mongoSession) readCount(
	ctx context.Context, parsed Statement,
) (db.QueryResult, error) {
	filter, err := ReadDocument(parsed.Calls[0].ReadArgument(0))
	if err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	counted, countErr := session.readCollection(parsed).CountDocuments(ctx, filter)
	if countErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(countErr)
	}
	return BuildValueResult("count", counted, 0, parsed.ReadMethod()), nil
}

// readDistinct returns the values one field holds.
func (session *mongoSession) readDistinct(
	ctx context.Context, parsed Statement, rowLimit int,
) (db.QueryResult, error) {
	call := parsed.Calls[0]
	field, err := ReadText(call.ReadArgument(0))
	if err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	filter, filterErr := ReadDocument(call.ReadArgument(1))
	if filterErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(filterErr)
	}

	var values bson.A
	if decodeErr := session.readCollection(parsed).
		Distinct(ctx, field, filter).Decode(&values); decodeErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(decodeErr)
	}

	rows := make([][]any, 0, len(values))
	held := ""
	for _, value := range values {
		held = joinTypes(held, ReadValueType(value))
		rows = append(rows, []any{FormatValue(value)})
	}
	return db.BuildCappedResult(db.CappedRead{
		Rows: rows, RowLimit: rowLimit, Command: "distinct",
		Columns: []db.ResultColumn{{Name: field, DataType: held}},
	}), nil
}

// readCursorUpTo reads the documents a cursor returns, and stops one document after the
// row limit. A pipeline answers with as many documents as it wants, and a cursor read to
// its end holds every one of them in memory. A row limit below zero reads them all.
func (session *mongoSession) readCursorUpTo(
	ctx context.Context, rowLimit int, open func() (*mongo.Cursor, error),
) ([]bson.D, error) {
	if rowLimit < 0 {
		return session.readCursor(ctx, open)
	}
	cursor, err := open()
	if err != nil {
		return nil, db.WrapDatabaseError(err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	wanted := db.ReadOverscanRowLimit(rowLimit)
	documents := []bson.D{}
	for len(documents) < wanted && cursor.Next(ctx) {
		document := bson.D{}
		if decodeErr := cursor.Decode(&document); decodeErr != nil {
			return nil, db.WrapDatabaseError(decodeErr)
		}
		documents = append(documents, document)
	}
	if cursorErr := cursor.Err(); cursorErr != nil {
		return nil, db.WrapDatabaseError(cursorErr)
	}
	return documents, nil
}

// readCursor reads every document a cursor returns.
func (session *mongoSession) readCursor(
	ctx context.Context, open func() (*mongo.Cursor, error),
) ([]bson.D, error) {
	cursor, err := open()
	if err != nil {
		return nil, db.WrapDatabaseError(err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	documents := []bson.D{}
	if allErr := cursor.All(ctx, &documents); allErr != nil {
		return nil, db.WrapDatabaseError(allErr)
	}
	return documents, nil
}

// writeInsert writes one document or a list of them.
func (session *mongoSession) writeInsert(
	ctx context.Context, parsed Statement,
) (db.QueryResult, error) {
	call := parsed.Calls[0]
	value, err := ReadValue(call.ReadArgument(0))
	if err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	collection := session.readCollection(parsed)

	if documents, isList := value.(bson.A); isList {
		answered, insertErr := collection.InsertMany(ctx, documents)
		if insertErr != nil {
			return db.QueryResult{}, db.WrapDatabaseError(insertErr)
		}
		return buildWriteResult(call.Name, int64(len(answered.InsertedIDs))), nil
	}
	if _, insertErr := collection.InsertOne(ctx, value); insertErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(insertErr)
	}
	return buildWriteResult(call.Name, 1), nil
}

// writeUpdate writes the documents a filter matches.
func (session *mongoSession) writeUpdate(
	ctx context.Context, parsed Statement,
) (db.QueryResult, error) {
	call := parsed.Calls[0]
	filter, err := ReadDocument(call.ReadArgument(0))
	if err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	change, changeErr := ReadValue(call.ReadArgument(1))
	if changeErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(changeErr)
	}
	collection := session.readCollection(parsed)

	var answered *mongo.UpdateResult
	var updateErr error
	switch call.Name {
	case "updateMany":
		answered, updateErr = collection.UpdateMany(ctx, filter, change)
	case "replaceOne":
		answered, updateErr = collection.ReplaceOne(ctx, filter, change)
	default:
		answered, updateErr = collection.UpdateOne(ctx, filter, change)
	}
	if updateErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(updateErr)
	}
	return buildWriteResult(call.Name, answered.ModifiedCount+answered.UpsertedCount), nil
}

// writeDelete removes the documents a filter matches.
func (session *mongoSession) writeDelete(
	ctx context.Context, parsed Statement,
) (db.QueryResult, error) {
	call := parsed.Calls[0]
	filter, err := ReadDocument(call.ReadArgument(0))
	if err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	collection := session.readCollection(parsed)

	var answered *mongo.DeleteResult
	var deleteErr error
	if call.Name == "deleteOne" || readsJustOne(call) {
		answered, deleteErr = collection.DeleteOne(ctx, filter)
	} else {
		answered, deleteErr = collection.DeleteMany(ctx, filter)
	}
	if deleteErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(deleteErr)
	}
	return buildWriteResult(call.Name, answered.DeletedCount), nil
}

// readsJustOne is true where a `remove` was asked to remove one document. The second
// argument of the call is the `justOne` of the old shell, which is either the flag itself
// or a document that holds it. A remove that names neither removes every match.
func readsJustOne(call MethodCall) bool {
	if call.Name != "remove" {
		return false
	}
	written := strings.TrimSpace(call.ReadArgument(1))
	if written == "" {
		return false
	}
	value, err := ReadValue(written)
	if err != nil {
		return false
	}
	if flag, isFlag := value.(bool); isFlag {
		return flag
	}
	document, isDocument := value.(bson.D)
	if !isDocument {
		return false
	}
	for _, element := range document {
		if element.Key == "justOne" {
			flag, isFlag := element.Value.(bool)
			return isFlag && flag
		}
	}
	return false
}

// writeFindAnd writes one document and returns the document itself.
func (session *mongoSession) writeFindAnd(
	ctx context.Context, parsed Statement,
) (db.QueryResult, error) {
	call := parsed.Calls[0]
	filter, err := ReadDocument(call.ReadArgument(0))
	if err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	collection := session.readCollection(parsed)

	var answered *mongo.SingleResult
	switch call.Name {
	case "findOneAndDelete":
		answered = collection.FindOneAndDelete(ctx, filter)
	default:
		change, changeErr := ReadValue(call.ReadArgument(1))
		if changeErr != nil {
			return db.QueryResult{}, db.WrapDatabaseError(changeErr)
		}
		if call.Name == "findOneAndReplace" {
			answered = collection.FindOneAndReplace(ctx, filter, change)
		} else {
			answered = collection.FindOneAndUpdate(ctx, filter, change)
		}
	}

	var document bson.D
	if decodeErr := answered.Decode(&document); decodeErr != nil {
		if decodeErr == mongo.ErrNoDocuments {
			return BuildDocumentResult(nil, 0, call.Name), nil
		}
		return db.QueryResult{}, db.WrapDatabaseError(decodeErr)
	}
	return BuildDocumentResult([]bson.D{document}, 0, call.Name), nil
}

// writeCreateIndex builds one index over the keys the call names.
func (session *mongoSession) writeCreateIndex(
	ctx context.Context, parsed Statement,
) (db.QueryResult, error) {
	call := parsed.Calls[0]
	keys, err := ReadDocument(call.ReadArgument(0))
	if err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	model := mongo.IndexModel{Keys: keys}
	if written := call.ReadArgument(1); written != "" {
		settings, settingsErr := ReadDocument(written)
		if settingsErr != nil {
			return db.QueryResult{}, db.WrapDatabaseError(settingsErr)
		}
		model.Options = buildIndexOptions(settings)
	}

	name, createErr := session.readCollection(parsed).Indexes().CreateOne(ctx, model)
	if createErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(createErr)
	}
	return BuildValueResult("created", name, 0, call.Name), nil
}

// buildIndexOptions reads the settings of an index that this client passes on.
func buildIndexOptions(settings bson.D) *options.IndexOptionsBuilder {
	held := options.Index()
	for _, field := range settings {
		switch field.Key {
		case "name":
			if name, isText := field.Value.(string); isText {
				held.SetName(name)
			}
		case "unique":
			if unique, isBool := field.Value.(bool); isBool {
				held.SetUnique(unique)
			}
		case "sparse":
			if sparse, isBool := field.Value.(bool); isBool {
				held.SetSparse(sparse)
			}
		}
	}
	return held
}

// buildWriteResult returns what a write changed, which is a count and no rows.
func buildWriteResult(command string, affected int64) db.QueryResult {
	return db.QueryResult{Command: command, Affected: affected, HasAffected: true}
}

// ReadPage returns one page of a read. The page is a skip and a limit over the find the
// read composed.
func (session *mongoSession) ReadPage(
	ctx context.Context, read db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	if !read.Pageable {
		return session.RunQuery(ctx, read.Text, window.Limit, read.Params)
	}
	startedAt := time.Now()
	parsed, err := ReadStatement(read.Text)
	if err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	plan, planErr := buildFindPlan(parsed)
	if planErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(planErr)
	}
	plan.skip += int64(window.Offset)

	bound, giveBack, holdErr := session.holdSession(ctx)
	if holdErr != nil {
		return db.QueryResult{}, holdErr
	}
	defer giveBack()

	documents, readErr := session.readCursor(bound, func() (*mongo.Cursor, error) {
		return session.readCollection(parsed).Find(
			bound, plan.filter, plan.buildFindOptions(window.Limit))
	})
	if readErr != nil {
		return db.QueryResult{}, session.noteServerFailure(readErr)
	}

	answered := BuildDocumentResult(documents, time.Since(startedAt), "find")
	return db.BuildCappedResult(db.CappedRead{
		Rows: answered.Rows, RowLimit: window.Limit, Columns: answered.Columns,
		Elapsed: time.Since(startedAt), Command: "find",
	}), nil
}

// CountRead counts the documents the read matches.
func (session *mongoSession) CountRead(
	ctx context.Context, read db.ComposedRead,
) (int64, bool, error) {
	if !read.Pageable {
		return 0, false, nil
	}
	parsed, err := ReadStatement(read.Text)
	if err != nil {
		return 0, false, db.WrapDatabaseError(err)
	}
	plan, planErr := buildFindPlan(parsed)
	if planErr != nil {
		return 0, false, db.WrapDatabaseError(planErr)
	}

	bound, giveBack, holdErr := session.holdSession(ctx)
	if holdErr != nil {
		return 0, false, holdErr
	}
	defer giveBack()

	counted, countErr := session.readCollection(parsed).CountDocuments(bound, plan.filter)
	if countErr != nil {
		return 0, false, session.noteServerFailure(db.WrapDatabaseError(countErr))
	}
	return counted, true, nil
}

// everyRow is the row limit that caps nothing, for a read that is written to a file
// rather than drawn.
const everyRow = -1

// StreamQuery reads a statement again for an export. A find and an aggregation are read a
// batch at a time, so an export never holds the whole collection. Every other statement
// returns one result, and that result is written, so the file holds what the grid showed
// rather than the documents behind it.
func (session *mongoSession) StreamQuery(
	ctx context.Context, buffer string, _ []any, batchSize int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	parsed, err := ReadStatement(buffer)
	if err != nil {
		return 0, db.WrapDatabaseError(err)
	}
	// An export runs the statement a second time, so only one that reads may be
	// repeated. A command is counted as a write, because a command can be anything.
	if resolveStatementRisk(buffer) != statement.RiskNone {
		return 0, db.NewDatabaseError(
			"an export runs the statement again, and only a read may be run twice")
	}
	if batchSize < 1 {
		batchSize = 1
	}

	bound, giveBack, holdErr := session.holdSession(ctx)
	if holdErr != nil {
		return 0, holdErr
	}
	defer giveBack()

	switch parsed.ReadMethod() {
	case "find", "findOne":
		plan, planErr := buildFindPlan(parsed)
		if planErr != nil {
			return 0, db.WrapDatabaseError(planErr)
		}
		return session.streamCursor(bound, batchSize, onBatch, func() (*mongo.Cursor, error) {
			return session.readCollection(parsed).Find(
				bound, plan.filter, plan.buildFindOptions(everyRow))
		})
	case "aggregate":
		pipeline, pipelineErr := ReadArray(parsed.Calls[0].ReadArgument(0))
		if pipelineErr != nil {
			return 0, db.WrapDatabaseError(pipelineErr)
		}
		return session.streamCursor(bound, batchSize, onBatch, func() (*mongo.Cursor, error) {
			return session.readCollection(parsed).Aggregate(bound, pipeline)
		})
	}

	answered, runErr := session.runStatement(bound, parsed, everyRow)
	if runErr != nil {
		return 0, session.noteServerFailure(runErr)
	}
	if len(answered.Rows) == 0 {
		return 0, nil
	}
	if batchErr := onBatch(answered.Rows, answered.Columns); batchErr != nil {
		return 0, batchErr
	}
	return int64(len(answered.Rows)), nil
}

// namedFieldLimit is how many left-out fields a message names before it counts the rest.
const namedFieldLimit = 5

// streamCursor writes the documents of a cursor a batch at a time. A file carries one
// header, so the columns are the fields of the first batch. A collection keeps no schema,
// so a later document can hold a field the header has not: those fields are counted and
// named at the end, because a file that quietly drops them reads as a whole one.
func (session *mongoSession) streamCursor(
	ctx context.Context, batchSize int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
	open func() (*mongo.Cursor, error),
) (int64, error) {
	cursor, err := open()
	if err != nil {
		return 0, db.WrapDatabaseError(err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	var columns []db.ResultColumn
	written := map[string]bool{}
	leftOut := []string{}
	batch := make([]bson.D, 0, batchSize)
	total := int64(0)

	write := func() error {
		if len(batch) == 0 {
			return nil
		}
		if columns == nil {
			columns = BuildDocumentColumns(batch)
			for _, column := range columns {
				written[column.Name] = true
			}
		}
		leftOut = append(leftOut, findLeftOutFields(batch, written)...)
		if batchErr := onBatch(BuildDocumentRows(batch, columns), columns); batchErr != nil {
			return batchErr
		}
		batch = batch[:0]
		return nil
	}

	for cursor.Next(ctx) {
		var document bson.D
		if decodeErr := cursor.Decode(&document); decodeErr != nil {
			return total, db.WrapDatabaseError(decodeErr)
		}
		batch = append(batch, document)
		total++
		if len(batch) < batchSize {
			continue
		}
		if writeErr := write(); writeErr != nil {
			return total, writeErr
		}
	}
	if cursorErr := cursor.Err(); cursorErr != nil {
		return total, db.WrapDatabaseError(cursorErr)
	}
	if writeErr := write(); writeErr != nil {
		return total, writeErr
	}
	if len(leftOut) > 0 {
		return total, buildLeftOutError(leftOut)
	}
	return total, nil
}

// findLeftOutFields returns the fields of these documents the header does not carry, each
// one once. It records them as it goes, so a later batch does not name them again.
func findLeftOutFields(documents []bson.D, written map[string]bool) []string {
	found := []string{}
	for _, document := range documents {
		for _, field := range document {
			if written[field.Key] {
				continue
			}
			written[field.Key] = true
			found = append(found, field.Key)
		}
	}
	return found
}

// buildLeftOutError reports the fields the file does not carry. The file itself was
// written, and says so, because the rows in it are the rows that were read.
func buildLeftOutError(leftOut []string) error {
	named := leftOut
	rest := 0
	if len(named) > namedFieldLimit {
		rest = len(named) - namedFieldLimit
		named = named[:namedFieldLimit]
	}
	written := strings.Join(named, ", ")
	if rest > 0 {
		written += fmt.Sprintf(", and %d more", rest)
	}
	return db.NewDatabaseError(
		"the file was written, and holds the fields of the first documents only: "+
			"%s appeared after those and are not in it", written)
}

// CheckStatement returns the fault this client finds in a statement. Only the server
// knows whether a field exists, and it says so by answering no documents.
func (session *mongoSession) CheckStatement(
	_ context.Context, written string,
) (db.StatementProblem, bool) {
	found := session.Support.Language.FindLocalDiagnostics(written, editor.NothingKnown())
	if len(found) == 0 {
		return db.StatementProblem{}, false
	}
	return db.StatementProblem{
		Message: found[0].Message, Offset: found[0].Start, HasOffset: true,
	}, true
}

// mongoAdapter opens a connection on a MongoDB server.
type mongoAdapter struct{ support db.EngineSupport }

// NewAdapter returns the adapter that opens a MongoDB server.
func NewAdapter(support db.EngineSupport) db.Adapter {
	return &mongoAdapter{support: support}
}

// BuildClientOptions returns the options the driver opens the profile with.
func BuildClientOptions(profile cfg.Profile, password string) *options.ClientOptions {
	held := options.Client().
		SetHosts([]string{fmt.Sprintf("%s:%d", profile.Host, profile.Port)}).
		SetAppName(applicationName).
		SetConnectTimeout(connectTimeout).
		SetServerSelectionTimeout(connectTimeout)

	if profile.User != "" {
		held.SetAuth(options.Credential{Username: profile.User, Password: password})
	}
	if config := db.BuildPolicyTLS(
		core.ResolveSSLPolicy(profile.SSLMode), profile.Host); config != nil {
		held.SetTLSConfig(config)
	}
	return held
}

// authenticationCodes are the codes the server returns where the connection is not
// allowed to run a command: it authenticated as nobody, or as a user with no rights.
var authenticationCodes = []int{
	13, // Unauthorized
	18, // AuthenticationFailed
}

// IsAuthenticationError is true where the server refused a command because of who the
// connection is.
func IsAuthenticationError(err error) bool {
	var reported mongo.ServerError
	if !errors.As(err, &reported) {
		return false
	}
	return slices.ContainsFunc(authenticationCodes, reported.HasErrorCode)
}

// BuildAuthenticationMessage writes why a server refused this profile. A profile that
// names no user is the common reason, and it says nothing on its own.
func BuildAuthenticationMessage(profile cfg.Profile, err error) string {
	if profile.User == "" {
		return fmt.Sprintf("cannot connect to %s: the server needs a user and this "+
			"profile names none", cfg.DescribeProfileTarget(profile))
	}
	return db.BuildConnectMessage(profile, err)
}

// lastServerError finds the reason a server was not reached. The driver reports that it
// chose no server and writes the whole topology after it, with the reason of each server
// buried inside. What the user needs is the reason.
var lastServerError = regexp.MustCompile(`Last error: ([^}]+?)\s*}`)

// DescribeConnectFailure writes why a connection could not be opened, with the reason of
// the server rather than the topology around it.
func DescribeConnectFailure(profile cfg.Profile, err error) string {
	found := lastServerError.FindStringSubmatch(err.Error())
	if found == nil {
		return db.BuildConnectMessage(profile, err)
	}
	return fmt.Sprintf("cannot connect to %s: %s",
		cfg.DescribeProfileTarget(profile), strings.TrimSpace(found[1]))
}

// ReadServerVersion reads the version out of the reply of buildInfo.
func ReadServerVersion(reply bson.D) string {
	for _, field := range reply {
		if field.Key == "version" {
			if written, isText := field.Value.(string); isText {
				return written
			}
		}
	}
	return "unknown"
}

func (adapter *mongoAdapter) Connect(
	ctx context.Context, profile cfg.Profile, password string,
) (db.Session, error) {
	client, err := mongo.Connect(BuildClientOptions(profile, password))
	if err != nil {
		return nil, db.WrapDatabaseMessage(DescribeConnectFailure(profile, err), err)
	}
	if pingErr := client.Ping(ctx, readpref.Primary()); pingErr != nil {
		_ = client.Disconnect(context.Background())
		return nil, db.WrapDatabaseMessage(DescribeConnectFailure(profile, pingErr), pingErr)
	}

	// A server with authentication turned on returns a ping without credentials and
	// refuses everything else, so the connection is checked with a command that has to
	// be authenticated. Without this a profile that gives the wrong credentials, or
	// none, opens and then fails on every read of it.
	version := "unknown"
	var reply bson.D
	buildErr := client.Database("admin").
		RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&reply)
	// Any other reason leaves the version unknown. A connection that opened is not
	// refused over a version.
	switch {
	case buildErr == nil:
		version = ReadServerVersion(reply)
	case IsAuthenticationError(buildErr):
		_ = client.Disconnect(context.Background())
		return nil, db.WrapDatabaseMessage(
			BuildAuthenticationMessage(profile, buildErr), buildErr)
	}

	// Whether a transaction can be held is a fact of the deployment, not of the engine,
	// so it is read from the server rather than assumed.
	var hello bson.D
	_ = client.Database("admin").
		RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello)

	database := strings.TrimSpace(profile.Database)
	if database == "" {
		database = defaultDatabase
	}
	return &mongoSession{
		SessionFacts: db.SessionFacts{
			Descriptor: db.SessionDescriptor{
				Profile: profile, ServerVersion: version, DefaultSchema: database,
			},
			Support: adapter.support,
		},
		client:            client,
		holdsTransactions: deploymentHoldsTransactions(hello),
		transaction:       newTransactionHolder(),
	}, nil
}

// countBatch returns the documents each collection holds, several at a time, so a tree
// of many collections is not read one call after another.
func (session *mongoSession) countBatch(ctx context.Context, tables []db.TableRef) {
	if len(tables) > countedCollections {
		return
	}
	places := make(chan int, len(tables))
	for at := range tables {
		places <- at
	}
	close(places)

	var waiting sync.WaitGroup
	for worker := 0; worker < countWorkers && worker < len(tables); worker++ {
		waiting.Go(func() {
			for at := range places {
				counted, err := session.client.
					Database(tables[at].Schema).Collection(tables[at].Name).
					EstimatedDocumentCount(ctx)
				if err == nil {
					tables[at].EstimatedRows = counted
				}
			}
		})
	}
	waiting.Wait()
}

// The compiler reports a part of the port this session has not answered for.
var _ db.Session = (*mongoSession)(nil)
