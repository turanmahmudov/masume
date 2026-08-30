package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The shell writes a document the way JavaScript writes an object. A client that read
// strict JSON only would refuse most of what a user pastes into it.
func TestReadValueReadsTheDocumentTheShellWrites(t *testing.T) {
	for _, held := range []struct {
		written string
		want    string
	}{
		{`{}`, `{}`},
		{`{status: "new"}`, `{"status":"new"}`},
		{`{'status': 'new'}`, `{"status":"new"}`},
		{`{total: {$gt: 10}}`, `{"total":{"$gt":10}}`},
		{`{a: 1, b: 2,}`, `{"a":1,"b":2}`},
		{`{count: 1e3}`, `{"count":1000.0}`},
		{`{open: true, closed: false, gone: null}`, `{"open":true,"closed":false,"gone":null}`},
		{`{name: /^ada/i}`, `{"name":{"$regularExpression":{"pattern":"^ada","options":"i"}}}`},
		{`{note: "a, b"}`, `{"note":"a, b"}`},
		// A comment is written for a reader and means nothing to the server.
		{"{a: 1} // the first", `{"a":1}`},
	} {
		value, err := ReadValue(held.written)
		if err != nil {
			t.Errorf("%q answered %v", held.written, err)
			continue
		}
		if written := WriteExtendedJSON(value); written != held.want {
			t.Errorf("%q reads as %s, wanted %s", held.written, written, held.want)
		}
	}
}

// An identity is pasted out of another client as ObjectId("…"), which the driver reads
// only in its extended form.
func TestReadValueReadsTheHelpersOfTheShell(t *testing.T) {
	value, err := ReadValue(`{_id: ObjectId("507f1f77bcf86cd799439011")}`)
	if err != nil {
		t.Fatalf("the document answered %v", err)
	}
	document, isDocument := value.(bson.D)
	if !isDocument || len(document) != 1 {
		t.Fatalf("the document reads as %#v", value)
	}
	held, isIdentity := document[0].Value.(bson.ObjectID)
	if !isIdentity {
		t.Fatalf("the identity reads as %#v", document[0].Value)
	}
	if held.Hex() != "507f1f77bcf86cd799439011" {
		t.Errorf("the identity reads %q", held.Hex())
	}
}

func TestReadValueRefusesWhatItCannotRead(t *testing.T) {
	for _, written := range []string{`{a: 1`, `{a: "never closes}`, `{a: /never closes}`} {
		if _, err := ReadValue(written); err == nil {
			t.Errorf("%q was read as a value", written)
		}
	}
}

// A statement names the database, the collection and the calls made on them, and every
// part of it decides what runs.
func TestParseStatementReadsTheCallChain(t *testing.T) {
	for _, held := range []struct {
		written    string
		database   string
		collection string
		method     string
		calls      int
	}{
		{`db.orders.find({})`, "", "orders", "find", 1},
		{`db.orders.find({}).sort({total: -1}).limit(10)`, "", "orders", "find", 3},
		{`db.getSiblingDB("shop").orders.find({})`, "shop", "orders", "find", 1},
		{`db.getCollection("order lines").find({})`, "", "order lines", "find", 1},
		{`db.runCommand({ping: 1})`, "", "", "runCommand", 1},
		{`db.orders.insertOne({total: 5});`, "", "orders", "insertOne", 1},
	} {
		parsed, fault, ok := ParseStatement(held.written)
		if !ok {
			t.Errorf("%q answered %q", held.written, fault.Message)
			continue
		}
		if parsed.Database != held.database {
			t.Errorf("%q names database %q, wanted %q",
				held.written, parsed.Database, held.database)
		}
		if parsed.Collection != held.collection {
			t.Errorf("%q names collection %q, wanted %q",
				held.written, parsed.Collection, held.collection)
		}
		if parsed.ReadMethod() != held.method {
			t.Errorf("%q calls %q, wanted %q", held.written, parsed.ReadMethod(), held.method)
		}
		if len(parsed.Calls) != held.calls {
			t.Errorf("%q holds %d calls, wanted %d",
				held.written, len(parsed.Calls), held.calls)
		}
	}
}

// A document holds a comma of its own, and a bracket, and both would end an argument if
// the reader counted them.
func TestParseStatementKeepsAnArgumentWhole(t *testing.T) {
	parsed, fault, ok := ParseStatement(`db.orders.updateOne({note: "a, b"}, {$set: {total: 5}})`)
	if !ok {
		t.Fatalf("the statement answered %q", fault.Message)
	}
	args := parsed.Calls[0].Args
	if len(args) != 2 {
		t.Fatalf("the call holds %d arguments, wanted two: %q", len(args), args)
	}
	if args[0] != `{note: "a, b"}` {
		t.Errorf("the filter reads %q", args[0])
	}
	if args[1] != `{$set: {total: 5}}` {
		t.Errorf("the change reads %q", args[1])
	}
}

func TestParseStatementNamesTheFaultItStoppedAt(t *testing.T) {
	for _, written := range []string{
		"",
		"orders.find({})",
		"db",
		"db.orders.find({}",
		"db.orders.find({}).",
	} {
		if _, fault, ok := ParseStatement(written); ok {
			t.Errorf("%q was read as a statement", written)
		} else if fault.Message == "" {
			t.Errorf("%q answered a fault with no message", written)
		}
	}
}
