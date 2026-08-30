package mongo

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/turanmahmudov/masume/internal/db"
)

// What the tree draws: the databases of the server, the collections of each, and the
// fields a sample of a collection holds. A collection keeps no schema, so the columns
// are read from the documents rather than from a catalog.

// ListTables returns every collection of every database the connection may read.
func (session *mongoSession) ListTables(ctx context.Context) ([]db.TableRef, error) {
	databases, err := session.listDatabaseNames(ctx)
	if err != nil {
		return nil, err
	}

	tables := []db.TableRef{}
	for _, name := range databases {
		specifications, listErr := session.client.Database(name).
			ListCollectionSpecifications(ctx, bson.D{})
		if listErr != nil {
			// A database the connection may not read is left out, because the tree of
			// the databases it may read is still the answer. Any other reason is not the
			// same as an empty database, so it is reported.
			if IsAuthenticationError(listErr) {
				continue
			}
			return nil, db.WrapDatabaseError(listErr)
		}
		for _, specification := range specifications {
			if IsSystemCollection(specification.Name) {
				continue
			}
			tables = append(tables, db.TableRef{
				Schema: name, Name: specification.Name,
				Kind: readRelationKind(specification.Type),
			})
		}
	}
	sort.Slice(tables, func(left, right int) bool {
		if tables[left].Schema != tables[right].Schema {
			return tables[left].Schema < tables[right].Schema
		}
		return tables[left].Name < tables[right].Name
	})

	session.countBatch(ctx, tables)
	return tables, nil
}

// listDatabaseNames returns the databases of the server, and the one of the connection
// alone where the connection may not list them.
func (session *mongoSession) listDatabaseNames(ctx context.Context) ([]string, error) {
	names, err := session.client.ListDatabaseNames(ctx, bson.D{})
	if err == nil {
		sort.Strings(names)
		return names, nil
	}
	// A user with rights on one database only cannot list the rest, and the tree of
	// that one database is still worth drawing.
	if _, listErr := session.client.Database(session.Descriptor.DefaultSchema).
		ListCollectionNames(ctx, bson.D{}); listErr != nil {
		return nil, db.WrapDatabaseError(err)
	}
	return []string{session.Descriptor.DefaultSchema}, nil
}

// systemCollectionPrefix marks the collections a database keeps for itself, inside a
// database of the user: the one that holds its views, the one that profiles it, and the
// buckets under a time series collection.
const systemCollectionPrefix = "system."

// IsSystemCollection is true where the database keeps this collection for itself.
func IsSystemCollection(name string) bool {
	return strings.HasPrefix(name, systemCollectionPrefix)
}

// readRelationKind returns the kind a collection is drawn as. A view is read and not
// written, and a time series collection is read like any other.
func readRelationKind(written string) db.RelationKind {
	if written == "view" {
		return db.RelationView
	}
	return db.RelationTable
}

// DescribeTable returns the fields a sample of the collection holds. A collection keeps
// no schema, so a field no document of the sample carries is not answered.
func (session *mongoSession) DescribeTable(
	ctx context.Context, table db.TableRef,
) (db.TableDetail, error) {
	documents, err := session.sampleDocuments(ctx, table)
	if err != nil {
		return db.TableDetail{}, err
	}

	names, types := collectDocumentFields(documents)
	columns := make([]db.ColumnDetail, 0, len(names)+1)
	held := false
	for _, name := range names {
		if name == IdentityField {
			held = true
		}
		columns = append(columns, db.ColumnDetail{
			Name: name, DataType: types[name], Nullable: true,
			IsPrimaryKey: name == IdentityField,
		})
	}
	// Every document has an identity, and an empty collection has no document to read
	// one from.
	if !held {
		columns = append([]db.ColumnDetail{{
			Name: IdentityField, DataType: TypeObjectID, IsPrimaryKey: true,
		}}, columns...)
	}
	return db.TableDetail{Table: table, Columns: columns}, nil
}

// sampleDocuments reads the head of a collection, which is what the fields are read from.
func (session *mongoSession) sampleDocuments(
	ctx context.Context, table db.TableRef,
) ([]bson.D, error) {
	return session.readCursor(ctx, func() (*mongo.Cursor, error) {
		return session.client.Database(table.Schema).Collection(table.Name).
			Find(ctx, bson.D{}, buildSampleOptions())
	})
}

// buildSampleOptions returns the options a sample of a collection is read with.
func buildSampleOptions() *options.FindOptionsBuilder {
	return options.Find().SetLimit(SampleSize)
}

// ListIndexes returns the indexes of one collection.
func (session *mongoSession) ListIndexes(
	ctx context.Context, table db.TableRef,
) ([]db.IndexDetail, error) {
	specifications, err := session.readIndexSpecifications(ctx, table)
	if err != nil {
		return nil, err
	}

	indexes := make([]db.IndexDetail, 0, len(specifications))
	for _, specification := range specifications {
		indexes = append(indexes, db.IndexDetail{
			Name:       specification.Name,
			IsUnique:   isSet(specification.Unique) || isIdentityIndex(specification),
			IsPrimary:  isIdentityIndex(specification),
			Definition: buildIndexDefinition(table, specification),
		})
	}
	return indexes, nil
}

// readIndexSpecifications returns the indexes of one collection as the driver reads them.
func (session *mongoSession) readIndexSpecifications(
	ctx context.Context, table db.TableRef,
) ([]mongo.IndexSpecification, error) {
	specifications, err := session.client.Database(table.Schema).
		Collection(table.Name).Indexes().ListSpecifications(ctx)
	if err != nil {
		return nil, db.WrapDatabaseError(err)
	}
	return specifications, nil
}

// readIndexDocuments returns the indexes of one collection as a result of its own, for
// the getIndexes call of a statement.
func (session *mongoSession) readIndexDocuments(
	ctx context.Context, parsed Statement,
) (db.QueryResult, error) {
	documents, err := session.readCursor(ctx, func() (*mongo.Cursor, error) {
		return session.readCollection(parsed).Indexes().List(ctx)
	})
	if err != nil {
		return db.QueryResult{}, err
	}
	return BuildDocumentResult(documents, 0, "getIndexes"), nil
}

// isIdentityIndex is true for the index every collection has over its identity.
func isIdentityIndex(specification mongo.IndexSpecification) bool {
	return specification.Name == IdentityField+"_"
}

func isSet(value *bool) bool { return value != nil && *value }

// buildIndexDefinition writes the call that builds this index again.
func buildIndexDefinition(
	table db.TableRef, specification mongo.IndexSpecification,
) string {
	settings := []string{`"name":` + strconv.Quote(specification.Name)}
	if isSet(specification.Unique) {
		settings = append(settings, `"unique":true`)
	}
	if isSet(specification.Sparse) {
		settings = append(settings, `"sparse":true`)
	}
	if specification.ExpireAfterSeconds != nil {
		settings = append(settings, fmt.Sprintf(
			`"expireAfterSeconds":%d`, *specification.ExpireAfterSeconds))
	}
	return BuildStatementText("", table.Name, fmt.Sprintf("createIndex(%s, {%s})",
		readIndexKeys(specification), strings.Join(settings, ",")))
}

// readIndexKeys writes the keys of an index as the document a call takes.
func readIndexKeys(specification mongo.IndexSpecification) string {
	var keys bson.D
	if err := bson.Unmarshal(specification.KeysDocument, &keys); err != nil {
		return "{}"
	}
	return WriteExtendedJSON(keys)
}

// ListConstraints returns what an index of a collection promises. MongoDB keeps no
// constraint of its own: an identity is unique because of its index, and so is every
// other unique field.
func (session *mongoSession) ListConstraints(
	ctx context.Context, table db.TableRef,
) ([]db.ConstraintDetail, error) {
	specifications, err := session.readIndexSpecifications(ctx, table)
	if err != nil {
		return nil, err
	}

	constraints := []db.ConstraintDetail{}
	for _, specification := range specifications {
		kind := db.ConstraintUnique
		switch {
		case isIdentityIndex(specification):
			kind = db.ConstraintPrimaryKey
		case !isSet(specification.Unique):
			continue
		}
		constraints = append(constraints, db.ConstraintDetail{
			Name: specification.Name, Kind: kind,
			Definition: readIndexKeys(specification),
		})
	}
	return constraints, nil
}

// ListRoles returns the users of the database of the connection. A deployment keeps its
// users in the admin database, which a connection may not read.
func (session *mongoSession) ListRoles(ctx context.Context) ([]db.DbRole, error) {
	var reply struct {
		Users []struct {
			User  string `bson:"user"`
			DB    string `bson:"db"`
			Roles []struct {
				Role string `bson:"role"`
				DB   string `bson:"db"`
			} `bson:"roles"`
		} `bson:"users"`
	}
	if err := session.readDatabase("").
		RunCommand(ctx, bson.D{{Key: "usersInfo", Value: 1}}).Decode(&reply); err != nil {
		return nil, db.WrapDatabaseError(err)
	}

	roles := make([]db.DbRole, 0, len(reply.Users))
	for _, user := range reply.Users {
		named := make([]string, 0, len(user.Roles))
		for _, role := range user.Roles {
			named = append(named, role.Role+"@"+role.DB)
		}
		roles = append(roles, db.DbRole{
			Name: user.User, Detail: strings.Join(named, ", "),
		})
	}
	return roles, nil
}

// BuildTableDDL writes the calls that build this collection and its indexes again. The
// index over the identity is left out, because every collection is created with one.
func (session *mongoSession) BuildTableDDL(
	ctx context.Context, table db.TableRef,
) ([]string, error) {
	lines := []string{BuildStatementText("", "",
		fmt.Sprintf("createCollection(%s)", strconv.Quote(table.Name)))}

	specifications, err := session.readIndexSpecifications(ctx, table)
	if err != nil {
		return nil, err
	}
	for _, specification := range specifications {
		if isIdentityIndex(specification) {
			continue
		}
		lines = append(lines, buildIndexDefinition(table, specification))
	}
	return lines, nil
}

// BuildObjectDDL answers with a comment line: a database holds no object of the kinds the
// tree groups, so there is no definition to write.
func (session *mongoSession) BuildObjectDDL(
	context.Context, db.SchemaObject,
) ([]string, error) {
	return []string{"// mongodb keeps no object of this kind"}, nil
}
