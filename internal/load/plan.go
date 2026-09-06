package load

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
)

// An import plan contains column mappings, row validation, and generated statements.

// SampleRows is the maximum rows per sample.
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
	// The first non-null sample value, displayed in the form.
	Example string
}

// Sample is the first file rows and their column types.
type Sample struct {
	Columns []SourceColumn
	Rows    []Row
	More    bool
}

// ReadSample reads the first file rows and infers their column types.
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

// buildSourceColumn returns the type, null count, and example for a sample column.
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
	// True if an insert can omit the column.
	Optional bool
	// True if the column accepts null.
	TakesNull bool
	// True for a server-generated column that imports cannot map.
	Generated bool
}

// Mapping is a source column and its target column. An empty target omits the source column.
type Mapping struct {
	Source string
	Target string
	// The kind the value is cast to before it is sent.
	Kind core.ColumnKind
}

// Plan is the import file, target table, and column mappings.
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

// BuildPlan maps matching column names and leaves unmatched source columns unmapped.
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

// MapColumn sets a column mapping and target type. An empty target removes the mapping.
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

// ListMappedColumns returns mapped columns in source order.
func (plan Plan) ListMappedColumns() []Mapping {
	mapped := make([]Mapping, 0, len(plan.Mappings))
	for _, mapping := range plan.Mappings {
		if mapping.Target != "" {
			mapped = append(mapped, mapping)
		}
	}
	return mapped
}

// FindPlanProblem returns a validation error, or an empty string for a valid plan.
func (plan Plan) FindPlanProblem(dialect *query.Dialect) string {
	if plan.Table.Name == "" {
		return "the table name is missing"
	}
	if len(plan.ListMappedColumns()) == 0 {
		return "no source columns are mapped to the table"
	}

	if held := len(plan.ListMappedColumns()); held > dialect.ResolveBindLimit() {
		return fmt.Sprintf(
			"the import has %d columns; the statement parameter limit is %d",
			held, dialect.ResolveBindLimit())
	}

	taken := map[string]bool{}
	for _, mapping := range plan.ListMappedColumns() {
		key := strings.ToLower(mapping.Target)
		if taken[key] {
			return fmt.Sprintf("multiple source columns are mapped to %q", mapping.Target)
		}
		taken[key] = true
	}

	for _, column := range plan.Target {
		if column.Optional || column.Generated || taken[strings.ToLower(column.Name)] {
			continue
		}
		return fmt.Sprintf("required column %s has no source mapping",
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

// CheckReport is the row count and validation errors from a dry run.
type CheckReport struct {
	Rows int
	// The rows that cannot be written. The list stops at MaxRowProblems.
	Problems []RowProblem
	Refused  int
}

// MaxRowProblems is the maximum reported row errors.
const MaxRowProblems = 20

// CheckFile reads the whole file and reports the rows the import cannot write.
func (plan Plan) CheckFile() (CheckReport, error) {
	report := CheckReport{}
	mapped := plan.ListMappedColumns()
	indexes := plan.buildSourceIndexes()

	// Later documents can add columns absent from the sample.
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

// findRowProblem returns the first row error, or false for a valid row.
func (plan Plan) findRowProblem(
	row Row, mapped []Mapping, indexes map[string]int, named int,
) (RowProblem, bool) {
	if len(row.Values) > named {
		return RowProblem{
			Line: row.Line,
			Reason: fmt.Sprintf("the row has %d fields; expected at most %d",
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
				Reason: mapping.Target + " does not accept null",
			}, true
		}
	}
	return RowProblem{}, false
}

// holdsNullFor is true if the target column accepts null.
func (plan Plan) holdsNullFor(target string) bool {
	if plan.CreatesTable {
		return true
	}
	held, found := findTargetColumn(plan.Target, target)
	return !found || held.TakesNull
}

// HoldsWritableRow is true if the row passes import validation.
func (plan Plan) HoldsWritableRow(row Row, named int) bool {
	_, refused := plan.findRowProblem(
		row, plan.ListMappedColumns(), plan.buildSourceIndexes(), named)
	return !refused
}

// appendProblem keeps errors up to MaxRowProblems.
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

// DescribeReport returns the row validation summary.
func DescribeReport(report CheckReport) string {
	if report.Refused == 0 {
		return fmt.Sprintf("%d rows; all rows passed validation", report.Rows)
	}
	return fmt.Sprintf("%d rows; %d rows failed validation",
		report.Rows, report.Refused)
}
