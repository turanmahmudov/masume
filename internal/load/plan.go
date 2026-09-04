package load

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
)

// The plan of one import: which column of the file goes into which column of the table,
// what every row would do, and the statements that write them.

// SampleRows is how many rows of the head of a file a sample reads.
const SampleRows = 200

// BatchRows is the most rows one insert writes.
const BatchRows = 1000

// ResolveBatchRows returns how many rows one insert writes.
func ResolveBatchRows(mapped int, dialect *query.Dialect) int {
	if mapped < 1 {
		return BatchRows
	}
	return max(min(dialect.ResolveBindLimit()/mapped, BatchRows), 1)
}

// SourceColumn is one column of the file.
type SourceColumn struct {
	Name   string
	Kind   core.ColumnKind
	Filled int
	Empty  int
	// The first value of the sample that is there, which the form shows as an example.
	Example string
}

// Sample is the head of a file, read to learn what it holds.
type Sample struct {
	Columns []SourceColumn
	Rows    []Row
	More    bool
}

// ReadSample reads the head of a file and returns what its columns hold.
func ReadSample(path string, options ReadOptions) (Sample, error) {
	sample := Sample{}
	names := []string{}
	values := [][]any{}

	err := WalkFile(path, options,
		func(read []string) error {
			names = read
			for len(values) < len(read) {
				values = append(values, nil)
			}
			return nil
		},
		func(row Row) error {
			if len(sample.Rows) >= SampleRows {
				sample.More = true
				return ErrStopWalk
			}
			sample.Rows = append(sample.Rows, row)
			for at := range values {
				if at < len(row.Values) {
					values[at] = append(values[at], row.Values[at])
				}
			}
			return nil
		})
	if err != nil && !isStopWalk(err) {
		return Sample{}, err
	}

	for at, name := range names {
		sample.Columns = append(sample.Columns, buildSourceColumn(name, values[at]))
	}
	return sample, nil
}

func isStopWalk(err error) bool { return err == ErrStopWalk }

// buildSourceColumn returns what one column of the sample holds.
func buildSourceColumn(name string, values []any) SourceColumn {
	column := SourceColumn{Name: name, Kind: ResolveColumnKind(values)}
	for _, value := range values {
		if value == nil {
			column.Empty++
			continue
		}
		column.Filled++
		if column.Example == "" {
			column.Example = core.FormatCell(value, "")
		}
	}
	return column
}

// TargetColumn is one column of the table the file is written into.
type TargetColumn struct {
	Name     string
	DataType string
	// True where a row that leaves the column out is written.
	Optional bool
	// True where the column holds a value that is not there.
	TakesNull bool
	// True where the server fills the column itself, so a mapping onto it is refused.
	Generated bool
}

// Mapping is what one column of the file does: the column of the table it is written into,
// or nothing where it is left out.
type Mapping struct {
	Source string
	Target string
	// The kind the value is cast to before it is sent.
	Kind core.ColumnKind
}

// Plan is one import as it stands: the file, the table, and what each column does.
type Plan struct {
	Path         string
	Options      ReadOptions
	Sample       Sample
	Table        query.QualifiedName
	Target       []TargetColumn
	CreatesTable bool
	Mappings     []Mapping
}

// findTargetColumn returns the column of the table of that name, read without regard to case.
func findTargetColumn(target []TargetColumn, name string) (TargetColumn, bool) {
	for _, column := range target {
		if strings.EqualFold(column.Name, name) {
			return column, true
		}
	}
	return TargetColumn{}, false
}

// BuildPlan returns the import of that sample into that table. A column of the file whose
// name a column of the table holds is mapped onto it, and any other column is left out for
// the user to map or to leave.
func BuildPlan(
	path string, options ReadOptions, sample Sample,
	table query.QualifiedName, target []TargetColumn,
) Plan {
	plan := Plan{
		Path: path, Options: options, Sample: sample, Table: table,
		Target: target, CreatesTable: len(target) == 0,
	}

	for _, column := range sample.Columns {
		mapping := Mapping{Source: column.Name, Kind: column.Kind}
		if plan.CreatesTable {
			mapping.Target = column.Name
		} else if held, found := findTargetColumn(target, column.Name); found && !held.Generated {
			mapping.Target = held.Name
			mapping.Kind = query.ReadTypeKind(held.DataType)
		}
		plan.Mappings = append(plan.Mappings, mapping)
	}
	return plan
}

// MapColumn writes one column of the file onto one column of the table, or leaves it out
// where the name is empty. The kind follows the column it is written into.
func (plan *Plan) MapColumn(source, target string) {
	for at, mapping := range plan.Mappings {
		if mapping.Source != source {
			continue
		}
		if target == "" {
			plan.Mappings[at].Target = ""
			return
		}
		plan.Mappings[at].Target = target
		plan.Mappings[at].Kind = plan.Sample.Columns[at].Kind
		if held, found := findTargetColumn(plan.Target, target); found {
			if held.Generated {
				plan.Mappings[at].Target = ""
				return
			}
			plan.Mappings[at].Target = held.Name
			plan.Mappings[at].Kind = query.ReadTypeKind(held.DataType)
		}
		return
	}
}

// ListMappedColumns returns the columns of the table the import writes, in the order the
// file holds them.
func (plan Plan) ListMappedColumns() []Mapping {
	mapped := make([]Mapping, 0, len(plan.Mappings))
	for _, mapping := range plan.Mappings {
		if mapping.Target != "" {
			mapped = append(mapped, mapping)
		}
	}
	return mapped
}

// FindPlanProblem returns why the import cannot run, and nothing where it can.
func (plan Plan) FindPlanProblem(dialect *query.Dialect) string {
	if plan.Table.Name == "" {
		return "the table cannot be empty"
	}
	if len(plan.ListMappedColumns()) == 0 {
		return "no column of the file is written into the table"
	}

	if held := len(plan.ListMappedColumns()); held > dialect.ResolveBindLimit() {
		return fmt.Sprintf(
			"the import writes %d columns and this server binds %d values to one statement",
			held, dialect.ResolveBindLimit())
	}

	taken := map[string]bool{}
	for _, mapping := range plan.ListMappedColumns() {
		key := strings.ToLower(mapping.Target)
		if taken[key] {
			return fmt.Sprintf("two columns of the file are written into %q", mapping.Target)
		}
		taken[key] = true
	}

	for _, column := range plan.Target {
		if column.Optional || column.Generated || taken[strings.ToLower(column.Name)] {
			continue
		}
		return fmt.Sprintf("%s takes no empty value and no column of the file fills it",
			column.Name)
	}
	return ""
}

// RowProblem is one row of the file the import cannot write, with the reason.
type RowProblem struct {
	Line   int
	Column string
	Reason string
}

// CheckReport is what a dry run found: how many rows the file holds, and the rows the
// import cannot write.
type CheckReport struct {
	Rows int
	// The rows that cannot be written. The list stops at MaxRowProblems.
	Problems []RowProblem
	Refused  int
}

// MaxRowProblems is how many refused rows a report lists.
const MaxRowProblems = 20

// CheckFile reads the whole file and reports the rows the import cannot write.
func (plan Plan) CheckFile() (CheckReport, error) {
	report := CheckReport{}
	mapped := plan.ListMappedColumns()
	indexes := plan.buildSourceIndexes()

	// A file of documents can name a column after the sample was read.
	named := len(plan.Sample.Columns)
	err := WalkFile(plan.Path, plan.Options,
		func(read []string) error {
			named = max(named, len(read))
			return nil
		},
		func(row Row) error {
			report.Rows++
			if problem, refused := plan.findRowProblem(
				row, mapped, indexes, named); refused {
				report.Refused++
				plan.appendProblem(&report, problem)
			}
			return nil
		})
	if err != nil {
		return CheckReport{}, err
	}
	return report, nil
}

// findRowProblem returns why one row cannot be written, and false where it can be.
func (plan Plan) findRowProblem(
	row Row, mapped []Mapping, indexes map[string]int, named int,
) (RowProblem, bool) {
	if len(row.Values) > named {
		return RowProblem{
			Line: row.Line,
			Reason: fmt.Sprintf("the row holds %d fields and the file names %d",
				len(row.Values), named),
		}, true
	}
	for _, mapping := range mapped {
		at := indexes[mapping.Source]
		var held any
		if at < len(row.Values) {
			held = row.Values[at]
		}
		value, err := CastValue(held, mapping.Kind)
		if err != nil {
			return RowProblem{
				Line: row.Line, Column: mapping.Source, Reason: err.Error(),
			}, true
		}
		if value == nil && !plan.holdsNullFor(mapping.Target) {
			return RowProblem{
				Line: row.Line, Column: mapping.Source,
				Reason: mapping.Target + " holds no empty value",
			}, true
		}
	}
	return RowProblem{}, false
}

// holdsNullFor is true where the column of the table takes a value that is not there.
func (plan Plan) holdsNullFor(target string) bool {
	if plan.CreatesTable {
		return true
	}
	held, found := findTargetColumn(plan.Target, target)
	return !found || held.TakesNull
}

// HoldsWritableRow is true where every value of the row can be written into the column it
// is mapped to.
func (plan Plan) HoldsWritableRow(row Row, named int) bool {
	_, refused := plan.findRowProblem(
		row, plan.ListMappedColumns(), plan.buildSourceIndexes(), named)
	return !refused
}

// appendProblem keeps the first problems and counts the rest.
func (plan Plan) appendProblem(report *CheckReport, problem RowProblem) {
	if len(report.Problems) < MaxRowProblems {
		report.Problems = append(report.Problems, problem)
	}
}

// buildSourceIndexes gives the position in a row of each column of the file.
func (plan Plan) buildSourceIndexes() map[string]int {
	indexes := map[string]int{}
	for at, column := range plan.Sample.Columns {
		indexes[column.Name] = at
	}
	return indexes
}

// DescribeReport returns what a dry run found, in one line.
func DescribeReport(report CheckReport) string {
	if report.Refused == 0 {
		return fmt.Sprintf("%d rows, and every one of them can be written", report.Rows)
	}
	return fmt.Sprintf("%d rows, and %d of them cannot be written",
		report.Rows, report.Refused)
}
