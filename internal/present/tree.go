package present

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// The ids of the three folders the tree draws around the server rows.
const (
	RolesID      = "roles"
	FavouritesID = "favourites"
	RecentID     = "recent"
)

// TreeNodeKind is the content type of one tree row.
type TreeNodeKind string

// The kinds of tree row.
const (
	NodeSchema     TreeNodeKind = "schema"
	NodeTable      TreeNodeKind = "table"
	NodeColumn     TreeNodeKind = "column"
	NodeCategory   TreeNodeKind = "category"
	NodeObject     TreeNodeKind = "object"
	NodeRoles      TreeNodeKind = "roles"
	NodeRole       TreeNodeKind = "role"
	NodeFavourites TreeNodeKind = "favourites"
	NodeRecent     TreeNodeKind = "recent"
	// A favourite whose table or schema no longer exists on the server.
	NodeLostFavourite TreeNodeKind = "lost-favourite"
)

// TreeNode is the object one tree row represents.
type TreeNode struct {
	Kind      TreeNodeKind
	Schema    string
	Table     db.TableRef
	Column    db.ColumnDetail
	Category  db.SchemaObjectKind
	Object    db.SchemaObject
	Role      db.DbRole
	Favourite core.Favourite
}

// TreeRow is one visible row of the tree.
type TreeRow struct {
	ID         string
	Depth      int
	Label      string
	Detail     string
	Expandable bool
	Expanded   bool
	Selectable bool
	// The kind of the row. The pane draws it as a glyph and a colour.
	Icon    cfg.IconKind
	HasIcon bool
	// True if the user marked this object.
	Marked bool
	Node   TreeNode
}

// TableIcons give the glyph of each kind of table.
var TableIcons = map[db.RelationKind]cfg.IconKind{
	db.RelationTable:            cfg.IconTable,
	db.RelationView:             cfg.IconView,
	db.RelationMaterializedView: cfg.IconMaterializedView,
}

// FindFavouriteOf returns the favourite the mark key sets on this node.
func FindFavouriteOf(node TreeNode) (core.Favourite, bool) {
	switch node.Kind {
	case NodeSchema:
		return core.Favourite{Kind: core.FavouriteSchema, Schema: node.Schema}, true
	case NodeTable:
		return core.Favourite{
			Kind: core.FavouriteTable, Schema: node.Table.Schema, Name: node.Table.Name,
		}, true
	case NodeLostFavourite:
		return node.Favourite, true
	}
	return core.Favourite{}, false
}

// Matches is true if the filter matches any of these fields.
func Matches(filter string, fields ...string) bool {
	if filter == "" {
		return true
	}
	for _, field := range fields {
		if MatchesSubsequence(field, filter) {
			return true
		}
	}
	return false
}

// rowOptions holds the values of one row, with the defaults of most rows.
type rowOptions struct {
	id         string
	depth      int
	label      string
	detail     string
	expandable bool
	expanded   bool
	selectable bool
	icon       cfg.IconKind
	hasIcon    bool
	marked     bool
	node       TreeNode
}

func buildRow(options rowOptions) TreeRow {
	return TreeRow{
		ID: options.id, Depth: options.depth, Label: options.label, Detail: options.detail,
		Expandable: options.expandable, Expanded: options.expanded,
		Selectable: options.selectable, Icon: options.icon, HasIcon: options.hasIcon,
		Marked: options.marked, Node: options.node,
	}
}

func withIcon(options rowOptions, icon cfg.IconKind) rowOptions {
	options.icon = icon
	options.hasIcon = true
	return options
}

// FormatAge returns the time since a schema was opened, short enough for the detail
// column.
func FormatAge(elapsed time.Duration) string {
	seconds := max(int64(math.Round(elapsed.Seconds())), 0)
	if seconds < 60 {
		return "just now"
	}
	minutes := int64(math.Round(float64(seconds) / 60))
	if minutes < 60 {
		return fmt.Sprintf("%dm ago", minutes)
	}
	hours := int64(math.Round(float64(minutes) / 60))
	if hours < 24 {
		return fmt.Sprintf("%dh ago", hours)
	}
	return fmt.Sprintf("%dd ago", int64(math.Round(float64(hours)/24)))
}

// treeFolder is one of the three folders the tree draws above and below the server rows.
type treeFolder struct {
	id    string
	label string
	icon  cfg.IconKind
	node  TreeNode
}

var (
	favouritesFolder = treeFolder{
		id: FavouritesID, label: "favourites", icon: cfg.IconFavourites,
		node: TreeNode{Kind: NodeFavourites},
	}
	recentFolder = treeFolder{
		id: RecentID, label: "recent", icon: cfg.IconRecent, node: TreeNode{Kind: NodeRecent},
	}
	rolesFolder = treeFolder{
		id: RolesID, label: "roles", icon: cfg.IconRoles, node: TreeNode{Kind: NodeRoles},
	}
)

// buildFolderRow returns the folder row of a group, open or closed.
func buildFolderRow(folder treeFolder, count int, open bool) TreeRow {
	return buildRow(withIcon(rowOptions{
		id: folder.id, label: folder.label, detail: strconv.Itoa(count),
		expandable: true, expanded: open, node: folder.node,
	}, folder.icon))
}

// buildFavouriteSchemaRow returns the row of a marked schema. A schema that no longer exists
// is marked as lost and is not removed.
func buildFavouriteSchemaRow(favourite core.Favourite, knownSchemas map[string]bool) TreeRow {
	known := knownSchemas[favourite.Schema]
	options := rowOptions{
		id: core.BuildFavouriteID(favourite), depth: 1, label: favourite.Schema,
		detail: "not found", selectable: known, marked: true,
		node: TreeNode{Kind: NodeLostFavourite, Favourite: favourite},
	}
	icon := cfg.IconNote
	if known {
		options.detail = "schema"
		options.node = TreeNode{Kind: NodeSchema, Schema: favourite.Schema}
		icon = cfg.IconSchema
	}
	return buildRow(withIcon(options, icon))
}

// buildFavouriteTableRow returns the row of a marked table. A table that no longer exists is
// marked as lost and is not removed.
func buildFavouriteTableRow(
	favourite core.Favourite, tablesByName map[string]db.TableRef,
) TreeRow {
	table, known := tablesByName[favourite.Schema+"."+favourite.Name]
	options := rowOptions{
		id: core.BuildFavouriteID(favourite), depth: 1,
		label: favourite.Schema + "." + favourite.Name, detail: "not found",
		selectable: known, marked: true,
		node: TreeNode{Kind: NodeLostFavourite, Favourite: favourite},
	}
	icon := cfg.IconNote
	if known {
		options.detail = FormatEstimatedRows(table.EstimatedRows)
		options.node = TreeNode{Kind: NodeTable, Table: table}
		icon = TableIcons[table.Kind]
	}
	return buildRow(withIcon(options, icon))
}

// buildFavouriteRows returns the rows of the objects the user marked, in the order of the
// marks.
func buildFavouriteRows(
	favourites []core.Favourite, knownSchemas map[string]bool,
	tablesByName map[string]db.TableRef, open bool,
) []TreeRow {
	if len(favourites) == 0 {
		return nil
	}
	rows := []TreeRow{buildFolderRow(favouritesFolder, len(favourites), open)}
	if !open {
		return rows
	}
	for _, favourite := range favourites {
		if favourite.Kind == core.FavouriteSchema {
			rows = append(rows, buildFavouriteSchemaRow(favourite, knownSchemas))
			continue
		}
		rows = append(rows, buildFavouriteTableRow(favourite, tablesByName))
	}
	return rows
}

// buildRecentRows returns the rows of the schemas opened recently, without a mark.
func buildRecentRows(recent []core.RecentSchema, open bool, now time.Time) []TreeRow {
	if len(recent) == 0 {
		return nil
	}
	rows := []TreeRow{buildFolderRow(recentFolder, len(recent), open)}
	if !open {
		return rows
	}
	for _, entry := range recent {
		detail := "schema"
		if !now.IsZero() {
			detail = FormatAge(now.Sub(entry.VisitedAt))
		}
		rows = append(rows, buildRow(withIcon(rowOptions{
			id: "recent:" + entry.Schema, depth: 1, label: entry.Schema, detail: detail,
			selectable: true, node: TreeNode{Kind: NodeSchema, Schema: entry.Schema},
		}, cfg.IconSchema)))
	}
	return rows
}

// buildRoleRows returns the rows of the roles of the server. A role is read and never
// opened.
func buildRoleRows(roles []db.DbRole, open bool) []TreeRow {
	if len(roles) == 0 {
		return nil
	}
	rows := []TreeRow{buildFolderRow(rolesFolder, len(roles), open)}
	if !open {
		return rows
	}
	for _, role := range roles {
		rows = append(rows, buildRow(withIcon(rowOptions{
			id: "role:" + role.Name, depth: 1, label: role.Name, detail: role.Detail,
			node: TreeNode{Kind: NodeRole, Role: role},
		}, cfg.IconRole)))
	}
	return rows
}

// categoryLabels give the folder of each kind of object.
var categoryLabels = map[db.SchemaObjectKind]string{
	db.ObjectFunction: "functions", db.ObjectSequence: "sequences",
	db.ObjectType: "types", db.ObjectTrigger: "triggers",
}

// categoryIcons give the glyph of each object folder.
var categoryIcons = map[db.SchemaObjectKind]cfg.IconKind{
	db.ObjectFunction: cfg.IconFunction, db.ObjectSequence: cfg.IconSequence,
	db.ObjectType: cfg.IconType, db.ObjectTrigger: cfg.IconTrigger,
}

// IsSystemSchema is true if the engine of the connection reserves that schema for itself.
func IsSystemSchema(schema string, engine core.Engine) bool {
	return core.ResolveEngineInfo(engine).HoldsSystemSchema(schema)
}

func buildCategoryID(schema string, category db.SchemaObjectKind) string {
	return "category:" + schema + ":" + string(category)
}

func resolveColumnIcon(column db.ColumnDetail, referencing map[string]bool) cfg.IconKind {
	if column.IsPrimaryKey {
		return cfg.IconPrimaryKey
	}
	if referencing[strings.ToLower(column.Name)] {
		return cfg.IconForeignKey
	}
	return cfg.IconColumn
}

// TableDetailStateKind is the state of the read of one table.
type TableDetailStateKind string

// The three states of the read of a table.
const (
	DetailLoading TableDetailStateKind = "loading"
	DetailReady   TableDetailStateKind = "ready"
	DetailFailed  TableDetailStateKind = "failed"
)

// TableDetailState is the state of the read of one table. It keeps the error, so a folded
// row can show it.
type TableDetailState struct {
	Kind    TableDetailStateKind
	Detail  db.TableDetail
	Message string
}

// BuildTableID returns the id of one table, so the data read for it can be found again.
func BuildTableID(table db.TableRef) string {
	return "table:" + table.Schema + "." + table.Name
}

// TreeSummary holds the counts the pane writes on its border: the visible rows and the
// hidden rows.
type TreeSummary struct {
	ShownSchemas        int
	TotalSchemas        int
	HiddenSystemSchemas int
}

// TreeResult holds the rows to draw and the summary of the border.
type TreeResult struct {
	Rows    []TreeRow
	Summary TreeSummary
}

// TreeInput holds every input of BuildTree.
type TreeInput struct {
	Tables  []db.TableRef
	Objects []db.SchemaObject
	Roles   []db.DbRole
	// The columns of each table after the read, keyed by the table row id.
	Details  map[string]TableDetailState
	Expanded map[string]bool
	Filter   string
	// A filter opened inside a schema searches that schema only. An empty scope searches
	// the whole tree.
	FilterScope       string
	Favourites        []core.Favourite
	Recent            []core.RecentSchema
	HideSystemSchemas bool
	Engine            core.Engine
	// The current time, for the age of a recent schema.
	Now time.Time
}

// treePlan holds the resolved input every row builder reads.
type treePlan struct {
	expanded map[string]bool
	details  map[string]TableDetailState
	// The favourite ids, so a marked row is drawn with its mark.
	marked map[string]bool
	// True while a filter searches the whole tree. Every searched folder is then open.
	globalFilter bool
	// The schema the filter was opened inside.
	scopedSchema string
}

// buildColumnRows returns the rows of the columns of an open table, or one row that reports
// the state of the read.
func buildColumnRows(plan treePlan, table db.TableRef, tableID string) []TreeRow {
	state, read := plan.details[tableID]
	if !read || state.Kind != DetailReady {
		label := "reading…"
		kind := DetailLoading
		if read {
			kind = state.Kind
		}
		options := rowOptions{
			id: tableID + ":" + string(kind), depth: 2, label: label,
			node: TreeNode{Kind: NodeTable, Table: table},
		}
		if kind == DetailFailed {
			options.label = "cannot read"
			return []TreeRow{buildRow(withIcon(options, cfg.IconNote))}
		}
		return []TreeRow{buildRow(options)}
	}

	referencing := map[string]bool{}
	for _, key := range state.Detail.ForeignKeys {
		for _, name := range key.Columns {
			referencing[strings.ToLower(name)] = true
		}
	}

	rows := make([]TreeRow, 0, len(state.Detail.Columns))
	for _, column := range state.Detail.Columns {
		rows = append(rows, buildRow(withIcon(rowOptions{
			id: "column:" + tableID + ":" + column.Name, depth: 2, label: column.Name,
			detail: AbbreviateDataType(column.DataType), selectable: true,
			node: TreeNode{Kind: NodeColumn, Table: table, Column: column},
		}, resolveColumnIcon(column, referencing))))
	}
	return rows
}

// buildTableRows returns the rows of every table of one schema, each one with its columns
// below it if the table is open.
func buildTableRows(plan treePlan, tables []db.TableRef) []TreeRow {
	rows := []TreeRow{}
	for _, table := range tables {
		tableID := BuildTableID(table)
		open := plan.expanded[tableID]

		rows = append(rows, buildRow(withIcon(rowOptions{
			id: tableID, depth: 1, label: table.Name,
			detail:     FormatEstimatedRows(table.EstimatedRows),
			expandable: true, expanded: open, selectable: true,
			marked: plan.marked[core.BuildFavouriteID(core.Favourite{
				Kind: core.FavouriteTable, Schema: table.Schema, Name: table.Name,
			})],
			node: TreeNode{Kind: NodeTable, Table: table},
		}, TableIcons[table.Kind])))

		if open {
			rows = append(rows, buildColumnRows(plan, table, tableID)...)
		}
	}
	return rows
}

// buildCategoryRows returns the object folders of a schema, each one with its objects if the
// folder is open.
func buildCategoryRows(
	plan treePlan, schema string, byKind map[db.SchemaObjectKind][]db.SchemaObject,
) []TreeRow {
	rows := []TreeRow{}
	for _, category := range db.SchemaObjectKinds {
		members := byKind[category]
		if len(members) == 0 {
			continue
		}

		categoryID := buildCategoryID(schema, category)
		open := plan.globalFilter || plan.expanded[categoryID]

		rows = append(rows, buildRow(withIcon(rowOptions{
			id: categoryID, depth: 1, label: categoryLabels[category],
			detail: strconv.Itoa(len(members)), expandable: true, expanded: open,
			node: TreeNode{Kind: NodeCategory, Schema: schema, Category: category},
		}, categoryIcons[category])))
		if !open {
			continue
		}

		for _, object := range members {
			rows = append(rows, buildRow(withIcon(rowOptions{
				id:    fmt.Sprintf("object:%s.%s:%s", object.Schema, object.Name, object.Kind),
				depth: 2, label: object.Name, detail: object.Detail,
				node: TreeNode{Kind: NodeObject, Object: object},
			}, categoryIcons[object.Kind])))
		}
	}
	return rows
}

// buildSchemaRows returns the rows of one schema, with its tables and its other objects
// below it if the schema is open.
func buildSchemaRows(
	plan treePlan, schema string, tables []db.TableRef,
	byKind map[db.SchemaObjectKind][]db.SchemaObject, hasObjects bool,
) []TreeRow {
	id := core.BuildSchemaID(schema)
	// A filter opens the folders it searched, so no match stays hidden.
	open := plan.globalFilter || schema == plan.scopedSchema || plan.expanded[id]

	row := buildRow(withIcon(rowOptions{
		id: id, label: schema, detail: strconv.Itoa(len(tables)),
		expandable: true, expanded: open, selectable: true,
		marked: plan.marked[core.BuildFavouriteID(core.Favourite{
			Kind: core.FavouriteSchema, Schema: schema,
		})],
		node: TreeNode{Kind: NodeSchema, Schema: schema},
	}, cfg.IconSchema))

	if !open {
		return []TreeRow{row}
	}
	rows := []TreeRow{row}
	rows = append(rows, buildTableRows(plan, tables)...)
	if hasObjects {
		rows = append(rows, buildCategoryRows(plan, schema, byKind)...)
	}
	return rows
}

// readSchemaFilter returns the filter that applies inside one schema. A filter opened on a
// schema searches that schema only. Any other scope searches the whole tree.
type readSchemaFilter func(schema string) string

// buildSchemaFilter returns the filter of each schema.
func buildSchemaFilter(filter, scopedSchema string, globalFilter bool) readSchemaFilter {
	return func(schema string) string {
		if globalFilter || schema == scopedSchema {
			return filter
		}
		return ""
	}
}

// groupTablesBySchema returns the matching tables of each schema, and every table by its
// full name, which the marked rows use. It runs in one pass, so the cost does not grow with
// the number of schemas times the number of tables.
func groupTablesBySchema(
	tables []db.TableRef, readFilterFor readSchemaFilter,
) (map[string][]db.TableRef, map[string]db.TableRef) {
	bySchema := map[string][]db.TableRef{}
	byName := map[string]db.TableRef{}
	for _, table := range tables {
		byName[table.Schema+"."+table.Name] = table
		if !Matches(readFilterFor(table.Schema), table.Name, table.Schema) {
			continue
		}
		bySchema[table.Schema] = append(bySchema[table.Schema], table)
	}
	return bySchema, byName
}

// groupObjectsBySchema returns the other matching objects of each schema, grouped by kind.
func groupObjectsBySchema(
	objects []db.SchemaObject, readFilterFor readSchemaFilter,
) map[string]map[db.SchemaObjectKind][]db.SchemaObject {
	bySchema := map[string]map[db.SchemaObjectKind][]db.SchemaObject{}
	for _, object := range objects {
		if !Matches(readFilterFor(object.Schema), object.Name, object.Detail) {
			continue
		}
		byKind, held := bySchema[object.Schema]
		if !held {
			byKind = map[db.SchemaObjectKind][]db.SchemaObject{}
			bySchema[object.Schema] = byKind
		}
		byKind[object.Kind] = append(byKind[object.Kind], object)
	}
	return bySchema
}

// collectSchemaNames returns every schema of the catalog, as a set and as an ordered list.
func collectSchemaNames(
	tables []db.TableRef, objects []db.SchemaObject,
) (map[string]bool, []string) {
	held := map[string]bool{}
	for _, table := range tables {
		held[table.Schema] = true
	}
	for _, object := range objects {
		held[object.Schema] = true
	}
	sorted := make([]string, 0, len(held))
	for schema := range held {
		sorted = append(sorted, schema)
	}
	slices.Sort(sorted)
	return held, sorted
}

// keepDrawnSchemas returns the schemas the tree draws, and the number of system schemas it
// skipped.
func keepDrawnSchemas(sorted []string, engine core.Engine, hideSystem bool) ([]string, int) {
	if !hideSystem {
		return sorted, 0
	}
	hidden := 0
	kept := make([]string, 0, len(sorted))
	for _, schema := range sorted {
		if IsSystemSchema(schema, engine) {
			hidden++
			continue
		}
		kept = append(kept, schema)
	}
	return kept, hidden
}

// keepKnownRecent returns the recently opened schemas that the tree still draws, up to the
// limit of the folder.
func keepKnownRecent(recent []core.RecentSchema, drawn []string) []core.RecentSchema {
	shown := map[string]bool{}
	for _, schema := range drawn {
		shown[schema] = true
	}
	kept := []core.RecentSchema{}
	for _, entry := range recent {
		if shown[entry.Schema] && len(kept) < core.RecentLimit {
			kept = append(kept, entry)
		}
	}
	return kept
}

// keepMatchingRoles returns the roles that match the filter.
func keepMatchingRoles(roles []db.DbRole, filter string) []db.DbRole {
	kept := make([]db.DbRole, 0, len(roles))
	for _, role := range roles {
		if Matches(filter, role.Name, role.Detail) {
			kept = append(kept, role)
		}
	}
	return kept
}

// collectMarkedIDs returns the id of every object the user marked, so a tree row can show
// its mark without a second read of the list.
func collectMarkedIDs(favourites []core.Favourite) map[string]bool {
	marked := map[string]bool{}
	for _, favourite := range favourites {
		marked[core.BuildFavouriteID(favourite)] = true
	}
	return marked
}

// buildSchemaGroupRows returns the rows of every schema that has content, and the number of
// schemas it drew. A search of the whole tree skips a schema without a match.
func buildSchemaGroupRows(
	plan treePlan, schemas []string, tablesBySchema map[string][]db.TableRef,
	objectsBySchema map[string]map[db.SchemaObjectKind][]db.SchemaObject,
) ([]TreeRow, int) {
	rows := []TreeRow{}
	drawn := 0
	for _, schema := range schemas {
		kept := tablesBySchema[schema]
		byKind, hasObjects := objectsBySchema[schema]
		if plan.globalFilter && len(kept) == 0 && !hasObjects {
			continue
		}
		drawn++
		rows = append(rows, buildSchemaRows(plan, schema, kept, byKind, hasObjects)...)
	}
	return rows, drawn
}

// BuildTree returns the rows to draw and the counts of the border.
func BuildTree(input TreeInput) TreeResult {
	filter := strings.TrimSpace(input.Filter)
	engine := input.Engine
	if engine == "" {
		engine = core.DefaultEngine
	}

	scopedSchema := core.FindSchemaOfID(input.FilterScope)
	globalFilter := filter != "" && scopedSchema == ""
	readFilterFor := buildSchemaFilter(filter, scopedSchema, globalFilter)

	tablesBySchema, tablesByName := groupTablesBySchema(input.Tables, readFilterFor)
	objectsBySchema := groupObjectsBySchema(input.Objects, readFilterFor)

	allSchemas, sortedSchemas := collectSchemaNames(input.Tables, input.Objects)
	schemas, hiddenSystem := keepDrawnSchemas(sortedSchemas, engine, input.HideSystemSchemas)
	knownRecent := keepKnownRecent(input.Recent, schemas)

	rows := []TreeRow{}
	rows = append(rows, buildFavouriteRows(
		input.Favourites, allSchemas, tablesByName, input.Expanded[FavouritesID])...)
	rows = append(rows, buildRecentRows(knownRecent, input.Expanded[RecentID], input.Now)...)

	plan := treePlan{
		expanded: input.Expanded, details: input.Details,
		marked:       collectMarkedIDs(input.Favourites),
		globalFilter: globalFilter, scopedSchema: scopedSchema,
	}
	schemaRows, drawnSchemas := buildSchemaGroupRows(
		plan, schemas, tablesBySchema, objectsBySchema)
	rows = append(rows, schemaRows...)

	roleFilter := ""
	if globalFilter {
		roleFilter = filter
	}
	rows = append(rows, buildRoleRows(
		keepMatchingRoles(input.Roles, roleFilter), globalFilter || input.Expanded[RolesID])...)

	return TreeResult{
		Rows: rows,
		Summary: TreeSummary{
			ShownSchemas: drawnSchemas, TotalSchemas: len(schemas),
			HiddenSystemSchemas: hiddenSystem,
		},
	}
}

// CollectDefaultExpanded opens one schema. Several schemas stay closed, because MySQL calls
// every database a schema. The marked and the recent folders are always open.
func CollectDefaultExpanded(
	tables []db.TableRef, objects []db.SchemaObject,
	favourites []core.Favourite, recent []core.RecentSchema,
) map[string]bool {
	open := map[string]bool{}
	if len(favourites) > 0 {
		open[FavouritesID] = true
	}
	if len(recent) > 0 {
		open[RecentID] = true
	}

	schemas := map[string]bool{}
	for _, table := range tables {
		schemas[table.Schema] = true
	}
	for _, object := range objects {
		schemas[object.Schema] = true
	}
	if len(schemas) == 1 {
		for schema := range schemas {
			open[core.BuildSchemaID(schema)] = true
		}
	}
	return open
}

// The guide line on the left of a tree. Without it the indent is the only indication of the
// depth, and the user has to count it.
const (
	// guideContinues marks a branch with more rows below this row at this depth.
	guideContinues = "│ "
	// guideLast marks the last row of a branch, which ends the line of its siblings.
	guideLast = "╰ "
	// guideClear marks a branch that ended above this row, so this depth draws nothing.
	guideClear = "  "
)

// buildGuide returns the prefix of one row, one cell per depth above it.
func buildGuide(depth int, open []bool) string {
	var guide strings.Builder
	for level := 1; level <= depth; level++ {
		if level < len(open) && open[level] {
			guide.WriteString(guideContinues)
			continue
		}
		if level < depth {
			guide.WriteString(guideClear)
			continue
		}
		guide.WriteString(guideLast)
	}
	return guide.String()
}

// markDepthOpen keeps this depth open and closes every deeper depth.
func markDepthOpen(open []bool, depth int) []bool {
	for len(open) <= depth {
		open = append(open, false)
	}
	open[depth] = true
	for deeper := depth + 1; deeper < len(open); deeper++ {
		open[deeper] = false
	}
	return open
}

// BuildTreeGuidesWithin returns the prefix of each row between two positions, in that order.
// The rows are read from the bottom to the top: a depth stays open while a row is at that
// depth, and every deeper depth closes at the next row above it. So the walk starts at the
// last row for every window, and only the prefixes inside the window are built. Before this,
// a pane of forty rows over an open catalog of thousands built every prefix.
func BuildTreeGuidesWithin(rows []TreeRow, from, to int) []string {
	from = max(from, 0)
	to = min(to, len(rows))
	if from >= to {
		return nil
	}

	guides := make([]string, to-from)
	open := []bool{}
	for index, row := range slices.Backward(rows) {
		depth := row.Depth
		if index >= from && index < to {
			guides[index-from] = buildGuide(depth, open)
		}
		open = markDepthOpen(open, depth)
	}
	return guides
}
