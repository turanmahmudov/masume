package mongo

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// Nothing here reaches the server. This turns a document into the rows of the grid, and
// the staged work of the grid into the commands that apply it.

// IdentityField names the field every document is identified by.
const IdentityField = "_id"

// SampleSize is how many documents are read to find the fields of a collection. A
// collection has no schema, so the columns are what a sample of it holds.
const SampleSize = 100

// The names of the types a value can have, as the server names them.
const (
	TypeObjectID = "objectId"
	TypeString   = "string"
	TypeInt      = "int"
	TypeLong     = "long"
	TypeDouble   = "double"
	TypeDecimal  = "decimal"
	TypeBool     = "bool"
	// A BSON date is a moment to the millisecond, not a day. The name says so, because
	// a column named `date` is drawn as a day alone and the time of it is lost.
	TypeDate      = "datetime"
	TypeTimestamp = "timestamp"
	TypeObject    = "object"
	TypeArray     = "array"
	TypeBinary    = "binData"
	TypeRegex     = "regex"
	TypeNull      = "null"
	// TypeMixed is a field the sample found under more than one type.
	TypeMixed = "mixed"
)

// FormatValue turns a value of a document into the value one cell holds. A structure
// becomes the text of its extended JSON, because a cell draws one line.
func FormatValue(value any) any {
	switch held := value.(type) {
	case nil, bson.Null:
		return nil
	case bson.ObjectID:
		return held.Hex()
	case bson.DateTime:
		return held.Time().UTC()
	case bson.Decimal128:
		return held.String()
	case bson.Binary:
		return held.Data
	case bson.Regex:
		return "/" + held.Pattern + "/" + held.Options
	case bson.Timestamp:
		return fmt.Sprintf("%d:%d", held.T, held.I)
	case bson.JavaScript:
		return string(held)
	case bson.Symbol:
		return string(held)
	case int32:
		return int64(held)
	case string, int64, float64, bool:
		return held
	}
	return WriteExtendedJSON(value)
}

// WriteExtendedJSON writes a value as the extended JSON a reader sees.
func WriteExtendedJSON(value any) string {
	written, err := bson.MarshalExtJSON(bson.D{{Key: "v", Value: value}}, false, false)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	// The value was wrapped, because extended JSON is written from a document only.
	text := string(written)
	text = strings.TrimSuffix(strings.TrimPrefix(text, `{"v":`), "}")
	return strings.TrimSpace(text)
}

// ReadValueType returns the name the server gives the type of this value.
func ReadValueType(value any) string {
	switch value.(type) {
	case nil, bson.Null:
		return TypeNull
	case bson.ObjectID:
		return TypeObjectID
	case bson.DateTime:
		return TypeDate
	case bson.Decimal128:
		return TypeDecimal
	case bson.Binary:
		return TypeBinary
	case bson.Regex:
		return TypeRegex
	case bson.Timestamp:
		return TypeTimestamp
	case bson.D, bson.M:
		return TypeObject
	case bson.A:
		return TypeArray
	case string:
		return TypeString
	case int32:
		return TypeInt
	case int64:
		return TypeLong
	case float64:
		return TypeDouble
	case bool:
		return TypeBool
	}
	return TypeMixed
}

// joinTypes returns the type of a column that has seen both of these. A column of one
// type keeps it; a column of several is mixed.
func joinTypes(held, found string) string {
	switch {
	case held == "" || held == TypeNull:
		return found
	case found == TypeNull || held == found:
		return held
	}
	return TypeMixed
}

// collectDocumentFields walks the documents once and returns the fields they hold, in the
// order they first appear, with the type of each.
func collectDocumentFields(documents []bson.D) ([]string, map[string]string) {
	names := []string{}
	types := map[string]string{}
	for _, document := range documents {
		for _, field := range document {
			if _, held := types[field.Key]; !held {
				names = append(names, field.Key)
			}
			types[field.Key] = joinTypes(types[field.Key], ReadValueType(field.Value))
		}
	}
	return names, types
}

// BuildDocumentColumns returns the columns a sample of documents holds.
func BuildDocumentColumns(documents []bson.D) []db.ResultColumn {
	names, types := collectDocumentFields(documents)
	columns := make([]db.ResultColumn, 0, len(names))
	for _, name := range names {
		columns = append(columns, db.ResultColumn{Name: name, DataType: types[name]})
	}
	return columns
}

// BuildDocumentRows lays the documents out under these columns. A field a document has
// not is answered as nothing, which is what a collection without a schema holds.
func BuildDocumentRows(documents []bson.D, columns []db.ResultColumn) [][]any {
	places := make(map[string]int, len(columns))
	for at, column := range columns {
		places[column.Name] = at
	}

	rows := make([][]any, 0, len(documents))
	for _, document := range documents {
		row := make([]any, len(columns))
		for _, field := range document {
			if at, held := places[field.Key]; held {
				row[at] = FormatValue(field.Value)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// BuildDocumentResult returns the documents as the columns and rows of a result.
func BuildDocumentResult(
	documents []bson.D, elapsed time.Duration, command string,
) db.QueryResult {
	columns := BuildDocumentColumns(documents)
	return db.QueryResult{
		Columns: columns, Rows: BuildDocumentRows(documents, columns),
		Elapsed: elapsed, Command: command,
	}
}

// BuildValueResult returns one value that is no document, such as a count or the reply
// of a command.
func BuildValueResult(name string, value any, elapsed time.Duration, command string) db.QueryResult {
	held := FormatValue(value)
	return db.QueryResult{
		Columns: []db.ResultColumn{{Name: name, DataType: ReadValueType(value)}},
		Rows:    [][]any{{held}}, Elapsed: elapsed, Command: command,
	}
}

// BuildStatementText writes a statement again, so a read the client composed reads as
// something the user could have typed.
func BuildStatementText(database, collection, call string) string {
	written := databaseWord
	if database != "" {
		written += "." + siblingCall + "(" + strconv.Quote(database) + ")"
	}
	if collection == "" {
		return written + "." + call
	}
	if isPlainName(collection) {
		return written + "." + collection + "." + call
	}
	return written + "." + collectionCall + "(" + strconv.Quote(collection) + ")." + call
}

// isPlainName is true for a collection the shell reads after a dot.
func isPlainName(name string) bool {
	if name == "" || !isNameStart(name[0]) || name[0] == '$' {
		return false
	}
	for index := 0; index < len(name); index++ {
		if !isNamePart(name[index]) || name[index] == '$' {
			return false
		}
	}
	return true
}

// BuildFilterText writes the filter of a tab as the document a find takes. Text the user
// typed is written in as it stands, and is read when the statement is read.
func BuildFilterText(filter []core.FilterStep) string {
	written := make([]string, 0, len(filter))
	for _, step := range filter {
		written = append(written, buildStepText(step))
	}
	switch len(written) {
	case 0:
		return "{}"
	case 1:
		return written[0]
	}
	return `{"$and":[` + strings.Join(written, ",") + "]}"
}

// buildStepText writes one step of the filter.
func buildStepText(step core.FilterStep) string {
	if step.Kind == core.FilterRaw {
		return step.Text
	}
	field := strconv.Quote(step.Column)
	switch step.Test {
	case core.FilterIsNull:
		return "{" + field + ":null}"
	case core.FilterIsNotNull:
		return "{" + field + `:{"$ne":null}}`
	case core.FilterDiffers:
		return "{" + field + `:{"$ne":` + writeMatchValue(step.Column, step.Value) + "}}"
	}
	return "{" + field + ":" + writeMatchValue(step.Column, step.Value) + "}"
}

// writeMatchValue writes the value a step compares with. The identity of a document is
// read back as its hexadecimal text, so the text is written again as the identity it
// stands for.
func writeMatchValue(column string, value any) string {
	if column == IdentityField {
		if text, isText := value.(string); isText {
			if _, err := bson.ObjectIDFromHex(text); err == nil {
				return `{"$oid":` + strconv.Quote(text) + "}"
			}
		}
	}
	return WriteExtendedJSON(value)
}

// BuildSortText writes the sort of a tab as the document a find takes.
func BuildSortText(sort []core.SortState) string {
	if len(sort) == 0 {
		return ""
	}
	written := make([]string, 0, len(sort))
	for _, key := range sort {
		direction := "1"
		if key.Direction == core.SortDescending {
			direction = "-1"
		}
		written = append(written, strconv.Quote(key.Column)+":"+direction)
	}
	return "{" + strings.Join(written, ",") + "}"
}

// ComposeRelationRead returns the find of one collection, with the sort and the filter
// of the tab written into it. The database is always named, so the statement reads the
// collection the tree names whichever database the connection opened.
func ComposeRelationRead(table db.TableRef, rewrite core.ReadRewrite) db.ComposedRead {
	call := "find(" + BuildFilterText(rewrite.Filter) + ")"
	if sort := BuildSortText(rewrite.Sort); sort != "" {
		call += ".sort(" + sort + ")"
	}
	text := BuildStatementText(table.Schema, table.Name, call)
	return db.ComposedRead{Text: text, Display: text, Pageable: true}
}

// ComposeStatementRead returns a statement of the user with the sort and the filter of
// the tab laid over it. Only a find takes them, and only one that pages itself is paged:
// a statement that names its own limit or skip already says which documents it wants.
func ComposeStatementRead(written db.BoundText, rewrite core.ReadRewrite) db.ComposedRead {
	plain := db.ComposedRead{Text: written.Text, Params: written.Params, Display: written.Text}

	parsed, _, ok := ParseStatement(written.Text)
	if !ok || parsed.ReadMethod() != "find" || parsed.Collection == "" {
		return plain
	}
	if _, held := parsed.FindCall("limit"); held {
		return plain
	}
	if _, held := parsed.FindCall("skip"); held {
		return plain
	}

	text := buildRewrittenFind(parsed, rewrite)
	return db.ComposedRead{Text: text, Display: text, Pageable: true}
}

// buildRewrittenFind writes the find again with the filter of the tab joined to its own,
// and the sort of the tab in place of its own.
func buildRewrittenFind(parsed Statement, rewrite core.ReadRewrite) string {
	find := parsed.Calls[0]
	args := append([]string{}, find.Args...)
	if len(args) == 0 {
		args = []string{"{}"}
	}
	args[0] = joinFilterText(args[0], rewrite.Filter)

	written := []string{writeCall(MethodCall{Name: find.Name, Args: args})}
	sort := BuildSortText(rewrite.Sort)
	for _, chained := range parsed.Calls[1:] {
		if chained.Name == "sort" && sort != "" {
			continue
		}
		written = append(written, writeCall(chained))
	}
	if sort != "" {
		written = append(written, "sort("+sort+")")
	}
	return BuildStatementText(parsed.Database, parsed.Collection, strings.Join(written, "."))
}

// joinFilterText joins the filter of the statement to the filter of the tab. Neither one
// is read here, so both are written as they stand.
func joinFilterText(written string, filter []core.FilterStep) string {
	held := BuildFilterText(filter)
	switch {
	case len(filter) == 0:
		return written
	case strings.TrimSpace(written) == "" || strings.TrimSpace(written) == "{}":
		return held
	}
	return `{"$and":[` + written + "," + held + "]}"
}

// writeCall writes one call of a statement again.
func writeCall(call MethodCall) string {
	return call.Name + "(" + strings.Join(call.Args, ", ") + ")"
}

// FindStatementSource returns the collection a statement of the user reads, so a row of
// its result can be written back. Only a find returns one: an aggregation reshapes the
// documents, and a command returns something that is no document of a collection at all.
func FindStatementSource(written string) (statement.SelectSource, bool) {
	parsed, _, ok := ParseStatement(written)
	if !ok || parsed.Collection == "" {
		return statement.SelectSource{}, false
	}
	if parsed.ReadMethod() != "find" && parsed.ReadMethod() != "findOne" {
		return statement.SelectSource{}, false
	}
	// A projection that drops the identity leaves the row unnameable. The columns of the
	// collection still say it has one, and the write is refused where the row has none.
	return statement.SelectSource{
		Schema: parsed.Database, HasSchema: parsed.Database != "", Name: parsed.Collection,
	}, true
}

// WriteKind is what one staged command does to a collection.
type WriteKind string

// The three kinds of write the grid stages.
const (
	WriteInsert WriteKind = "insert"
	WriteUpdate WriteKind = "update"
	WriteDelete WriteKind = "delete"
)

// WriteCommand is one command a staged change carries.
type WriteCommand struct {
	Database   string
	Collection string
	Kind       WriteKind
	Filter     bson.D
	Document   bson.D
}

// ReadWriteCommand reads back the command a change carries. A change built by another
// engine is refused, not sent to the server.
func ReadWriteCommand(change db.Change) (WriteCommand, error) {
	command, built := change.Payload.(WriteCommand)
	if !built {
		return WriteCommand{}, core.NewEditError("this change was not built for this session")
	}
	return command, nil
}

// BuildWriteValue reads what the user typed in a cell as the value the field takes. The
// type of the column decides how the text is read, because a collection has no schema to
// ask.
func BuildWriteValue(value core.CellValue, dataType string) (any, error) {
	if value.Kind == core.CellNull || value.Kind == core.CellDefault {
		return nil, nil
	}
	written := value.Text
	if value.Kind == core.CellEmpty {
		written = ""
	}

	switch dataType {
	case TypeString:
		return written, nil
	case TypeObjectID:
		held, err := bson.ObjectIDFromHex(strings.TrimSpace(written))
		if err != nil {
			return nil, core.NewEditError("an identity is twenty-four hexadecimal characters")
		}
		return held, nil
	case TypeInt, TypeLong:
		held, err := strconv.ParseInt(strings.TrimSpace(written), 10, 64)
		if err != nil {
			return nil, core.NewEditError("this field holds a whole number")
		}
		if dataType == TypeInt {
			return int32(held), nil
		}
		return held, nil
	case TypeDouble:
		held, err := strconv.ParseFloat(strings.TrimSpace(written), 64)
		if err != nil {
			return nil, core.NewEditError("this field holds a number")
		}
		return held, nil
	case TypeDecimal:
		held, err := bson.ParseDecimal128(strings.TrimSpace(written))
		if err != nil {
			return nil, core.NewEditError("this field holds a decimal number")
		}
		return held, nil
	case TypeBool:
		held, err := strconv.ParseBool(strings.TrimSpace(written))
		if err != nil {
			return nil, core.NewEditError("this field holds true or false")
		}
		return held, nil
	case TypeDate:
		held, err := ReadDateText(written)
		if err != nil {
			return nil, err
		}
		return bson.NewDateTimeFromTime(held), nil
	case TypeObject, TypeArray:
		held, err := ReadValue(written)
		if err != nil {
			return nil, core.NewEditError("this field holds a document: " + err.Error())
		}
		return held, nil
	}

	// A field the sample saw under several types, or none, is read as the value it
	// writes and left as text where it is no value.
	held, err := ReadValue(written)
	if err != nil {
		return written, nil
	}
	return held, nil
}

// dateLayouts are the forms a date is read from: the one the grid writes, and the two a
// user types.
var dateLayouts = []string{
	time.RFC3339Nano, "2006-01-02 15:04:05.000", "2006-01-02 15:04:05", "2006-01-02",
}

// ReadDateText reads the text of a date as the moment it names, in UTC.
func ReadDateText(written string) (time.Time, error) {
	trimmed := strings.TrimSpace(written)
	for _, layout := range dateLayouts {
		if held, err := time.Parse(layout, trimmed); err == nil {
			return held.UTC(), nil
		}
	}
	return time.Time{}, core.NewEditError("a date reads as 2006-01-02 15:04:05.000")
}

// BuildIdentityFilter returns the filter that names one document.
func BuildIdentityFilter(value any, dataType string) (bson.D, error) {
	if value == nil {
		return nil, core.NewEditError("this row has no identity, so it cannot be written")
	}
	if dataType == TypeObjectID {
		text, isText := value.(string)
		if !isText {
			return nil, core.NewEditError("the identity of this row is not readable")
		}
		held, err := bson.ObjectIDFromHex(text)
		if err != nil {
			return nil, core.NewEditError("the identity of this row is not readable")
		}
		return bson.D{{Key: IdentityField, Value: held}}, nil
	}
	return bson.D{{Key: IdentityField, Value: value}}, nil
}

// changeTarget holds what every command of one staged set writes to.
type changeTarget struct {
	database   string
	collection string
	columns    []db.ResultColumn
	rows       [][]any
}

// findIdentity returns the filter that names the row at that place.
func (target changeTarget) findIdentity(rowIndex int) (bson.D, string, error) {
	at := -1
	for index, column := range target.columns {
		if column.Name == IdentityField {
			at = index
			break
		}
	}
	if at == -1 {
		return nil, "", core.NewEditError(
			"this read answers no " + IdentityField + ", so no document can be named")
	}
	if rowIndex < 0 || rowIndex >= len(target.rows) || at >= len(target.rows[rowIndex]) {
		return nil, "", core.NewEditError("this row is no longer on the screen")
	}
	value := target.rows[rowIndex][at]
	filter, err := BuildIdentityFilter(value, target.columns[at].DataType)
	if err != nil {
		return nil, "", err
	}
	return filter, db.ReadAnyText(value), nil
}

// findColumnType returns the type the sample gave that column.
func (target changeTarget) findColumnType(columnIndex int) (string, string) {
	if columnIndex < 0 || columnIndex >= len(target.columns) {
		return "", ""
	}
	return target.columns[columnIndex].Name, target.columns[columnIndex].DataType
}

// BuildChanges returns the commands that apply what the grid staged.
func BuildChanges(
	target db.ChangeTarget, staged core.PendingChanges,
) ([]db.Change, error) {
	held := changeTarget{
		database: target.Table.Schema, collection: target.Table.Name,
		columns: target.Columns, rows: target.Rows,
	}

	changes := []db.Change{}
	for _, values := range staged.Inserts {
		change, err := held.buildInsert(values)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}

	updates, err := held.buildUpdates(staged)
	if err != nil {
		return nil, err
	}
	changes = append(changes, updates...)

	deletes, err := held.buildDeletes(staged)
	if err != nil {
		return nil, err
	}
	return append(changes, deletes...), nil
}

// buildInsert returns the command that writes one new document. The form is read as
// JSON, so a value already carries its own type and only the text of one is read again.
func (target changeTarget) buildInsert(values map[string]any) (db.Change, error) {
	document := bson.D{}
	taken := map[string]bool{}
	for _, column := range target.columns {
		written, staged := values[column.Name]
		if !staged {
			continue
		}
		taken[column.Name] = true
		value, err := BuildInsertValue(written, column.DataType)
		if err != nil {
			return db.Change{}, err
		}
		document = append(document, bson.E{Key: column.Name, Value: value})
	}

	// A field the sample never saw is still written, because a collection holds
	// whatever a document gives it.
	extra := make([]string, 0, len(values))
	for name := range values {
		if !taken[name] {
			extra = append(extra, name)
		}
	}
	slices.Sort(extra)
	for _, name := range extra {
		value, err := BuildInsertValue(values[name], "")
		if err != nil {
			return db.Change{}, err
		}
		document = append(document, bson.E{Key: name, Value: value})
	}

	if len(document) == 0 {
		return db.Change{}, core.NewEditError("a new document needs at least one field")
	}
	return target.buildChange(WriteCommand{
		Kind: WriteInsert, Document: document,
	}, "insertOne", "insert one document into "+target.collection), nil
}

// BuildInsertValue reads one value of a new document. A value that came from JSON or
// from a row already on the screen keeps its type; text is read as the type of the
// column.
func BuildInsertValue(written any, dataType string) (any, error) {
	switch held := written.(type) {
	case nil:
		return nil, nil
	case string:
		return BuildWriteValue(core.CellValue{Kind: core.CellText, Text: held}, dataType)
	case float64:
		switch dataType {
		case TypeInt:
			return int32(held), nil
		case TypeLong:
			return int64(held), nil
		}
		return held, nil
	case time.Time:
		return bson.NewDateTimeFromTime(held), nil
	case []byte:
		return bson.Binary{Data: held}, nil
	}
	return written, nil
}

// buildUpdates returns one command per row the grid edited, whatever the number of cells
// edited in it.
func (target changeTarget) buildUpdates(staged core.PendingChanges) ([]db.Change, error) {
	order := []int{}
	byRow := map[int]bson.D{}

	for _, edit := range core.SortedEdits(staged) {
		if staged.DeletedRows[edit.RowIndex] {
			continue
		}
		name, dataType := target.findColumnType(edit.ColumnIndex)
		if name == "" {
			return nil, core.NewEditError("this column is no longer on the screen")
		}
		if name == IdentityField {
			return nil, core.NewEditError(
				"the " + IdentityField + " of a document cannot be written")
		}
		value, err := BuildWriteValue(edit.Value, dataType)
		if err != nil {
			return nil, err
		}
		if _, held := byRow[edit.RowIndex]; !held {
			order = append(order, edit.RowIndex)
		}
		byRow[edit.RowIndex] = append(byRow[edit.RowIndex], bson.E{Key: name, Value: value})
	}

	changes := make([]db.Change, 0, len(order))
	for _, rowIndex := range order {
		filter, identity, err := target.findIdentity(rowIndex)
		if err != nil {
			return nil, err
		}
		changes = append(changes, target.buildChange(
			WriteCommand{
				Kind: WriteUpdate, Filter: filter,
				Document: bson.D{{Key: "$set", Value: byRow[rowIndex]}},
			},
			"updateOne", "update "+identity))
	}
	return changes, nil
}

// buildDeletes returns one command per row the grid marked.
func (target changeTarget) buildDeletes(staged core.PendingChanges) ([]db.Change, error) {
	deleted := core.SortedDeletedRows(staged)
	changes := make([]db.Change, 0, len(deleted))
	for _, rowIndex := range deleted {
		filter, identity, err := target.findIdentity(rowIndex)
		if err != nil {
			return nil, err
		}
		changes = append(changes, target.buildChange(
			WriteCommand{Kind: WriteDelete, Filter: filter},
			"deleteOne", "delete "+identity))
	}
	return changes, nil
}

// buildChange returns the change the grid shows and the session applies.
func (target changeTarget) buildChange(
	command WriteCommand, method, description string,
) db.Change {
	command.Database = target.database
	command.Collection = target.collection

	written := []string{}
	if command.Filter != nil {
		written = append(written, WriteExtendedJSON(command.Filter))
	}
	if command.Document != nil {
		written = append(written, WriteExtendedJSON(command.Document))
	}
	return db.Change{
		Description: description,
		Display: BuildStatementText(target.database, target.collection,
			method+"("+strings.Join(written, ", ")+")"),
		Payload: command,
	}
}
