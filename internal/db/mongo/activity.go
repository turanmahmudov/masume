package mongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/turanmahmudov/masume/internal/db"
)

// The connections of the server itself. $currentOp lists what is running, and killOp
// ends one of them.

// ListActivity returns what the server is running now.
func (session *mongoSession) ListActivity(ctx context.Context) ([]db.Activity, error) {
	admin := session.client.Database("admin")
	documents, err := session.readCursor(ctx, func() (*mongo.Cursor, error) {
		return admin.Aggregate(ctx, bson.A{
			bson.D{{Key: "$currentOp", Value: bson.D{
				{Key: "allUsers", Value: true}, {Key: "idleConnections", Value: false},
			}}},
		})
	})
	if err != nil {
		return nil, err
	}

	activity := make([]db.Activity, 0, len(documents))
	for _, document := range documents {
		activity = append(activity, ReadOperation(document))
	}
	return activity, nil
}

// ReadOperation reads one running operation as the activity list holds it.
func ReadOperation(document bson.D) db.Activity {
	fields := map[string]any{}
	for _, field := range document {
		fields[field.Key] = field.Value
	}

	entry := db.Activity{
		PID:             db.ReadNonNegativeCount(fields["opid"]),
		User:            readOperationUser(fields["effectiveUsers"]),
		ApplicationName: db.ReadAnyText(fields["appName"]),
		ClientAddress:   db.ReadAnyText(fields["client"]),
		State:           db.ReadAnyText(fields["op"]),
		Duration:        readOperationDuration(fields),
		Query:           readOperationCommand(fields),
	}
	if entry.State == "" {
		entry.State = db.ReadAnyText(fields["desc"])
	}
	return entry
}

// readOperationUser returns the user an operation runs as, which the server returns as
// a list.
func readOperationUser(value any) string {
	users, isList := value.(bson.A)
	if !isList || len(users) == 0 {
		return ""
	}
	held, isDocument := users[0].(bson.D)
	if !isDocument {
		return ""
	}
	for _, field := range held {
		if field.Key == "user" {
			return db.ReadAnyText(field.Value)
		}
	}
	return ""
}

// readOperationDuration returns how long an operation has been running. The server
// reports it in microseconds, and in whole seconds where it does not.
func readOperationDuration(fields map[string]any) time.Duration {
	if held, reported := fields["microsecs_running"]; reported {
		return time.Duration(db.ReadNonNegativeCount(held)) * time.Microsecond
	}
	return time.Duration(db.ReadNonNegativeCount(fields["secs_running"])) * time.Second
}

// readOperationCommand writes the command an operation is running.
func readOperationCommand(fields map[string]any) string {
	command, reported := fields["command"]
	if !reported {
		return db.ReadAnyText(fields["ns"])
	}
	return WriteExtendedJSON(command)
}

// CancelBackend ends one running operation. The server has no second word for
// terminating, so a kill is a kill.
func (session *mongoSession) CancelBackend(
	ctx context.Context, pid int64, _ bool,
) (bool, error) {
	var reply bson.D
	err := session.client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "killOp", Value: 1}, {Key: "op", Value: pid},
	}).Decode(&reply)
	if err != nil {
		return false, db.WrapDatabaseError(err)
	}
	return true, nil
}

// CancelRunningQuery is refused: the client stops waiting through its own context, and no
// second connection knows the operation id to kill.
func (session *mongoSession) CancelRunningQuery(context.Context) (bool, error) {
	return false, db.NewUnsupportedError("cancel a running statement")
}
