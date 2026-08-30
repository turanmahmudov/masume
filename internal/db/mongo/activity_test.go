package mongo

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The activity list names one running operation, and the number it names is the one a
// kill takes.
func TestReadOperationReadsWhatTheServerIsRunning(t *testing.T) {
	entry := ReadOperation(bson.D{
		{Key: "opid", Value: int64(4321)},
		{Key: "client", Value: "127.0.0.1:55123"},
		{Key: "appName", Value: "masume"},
		{Key: "op", Value: "query"},
		{Key: "microsecs_running", Value: int64(1_500_000)},
		{Key: "effectiveUsers", Value: bson.A{
			bson.D{{Key: "user", Value: "reader"}, {Key: "db", Value: "admin"}},
		}},
		{Key: "command", Value: bson.D{{Key: "find", Value: "orders"}}},
	})

	if entry.PID != 4321 {
		t.Errorf("the operation is numbered %d", entry.PID)
	}
	if entry.User != "reader" {
		t.Errorf("the user reads %q", entry.User)
	}
	if entry.ClientAddress != "127.0.0.1:55123" {
		t.Errorf("the address reads %q", entry.ClientAddress)
	}
	if entry.Duration != 1500*time.Millisecond {
		t.Errorf("the operation has run for %v, wanted 1.5s", entry.Duration)
	}
	if entry.Query != `{"find":"orders"}` {
		t.Errorf("the command reads %q", entry.Query)
	}
}

// An older server reports whole seconds only, and a connection that is running nothing
// names itself instead of an operation.
func TestReadOperationReadsWhatAThinnerReplyHolds(t *testing.T) {
	entry := ReadOperation(bson.D{
		{Key: "opid", Value: int32(7)},
		{Key: "secs_running", Value: int32(3)},
		{Key: "desc", Value: "conn12"},
		{Key: "ns", Value: "shop.orders"},
	})

	if entry.PID != 7 {
		t.Errorf("the operation is numbered %d", entry.PID)
	}
	if entry.Duration != 3*time.Second {
		t.Errorf("the operation has run for %v, wanted 3s", entry.Duration)
	}
	if entry.State != "conn12" {
		t.Errorf("the state reads %q, wanted the name of the connection", entry.State)
	}
	if entry.Query != "shop.orders" {
		t.Errorf("the query reads %q, wanted what it is reading", entry.Query)
	}
}
