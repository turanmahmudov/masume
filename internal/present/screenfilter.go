package present

import (
	"fmt"
	"hash/fnv"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// A filter over the rows on screen. It hides rows and reads none, so its counts are the
// counts of the page and not of the table. A WHERE clause filters the whole table.

// ValueCount is one value of a column and the number of rows on screen that have it.
type ValueCount struct {
	Value string
	Count int
}

// ScreenFilter is the filter of the grid: the kept values per column, and a search term over
// every column.
type ScreenFilter struct {
	// The kept values per column. A column without an entry is not filtered.
	Values map[int]map[string]bool
	Search string
}

// NoScreenFilter hides no row.
func NoScreenFilter() ScreenFilter {
	return ScreenFilter{Values: map[int]map[string]bool{}}
}

// IsEmpty is true if the filter hides no row.
func (filter ScreenFilter) IsEmpty() bool {
	return len(filter.Values) == 0 && filter.Search == ""
}

// CountColumnValues returns the values of one column, the most frequent first, and then in
// the order they were found.
func CountColumnValues(rows [][]string, columnIndex int) []ValueCount {
	counts := map[string]int{}
	order := []string{}
	for _, row := range rows {
		value := ""
		if columnIndex >= 0 && columnIndex < len(row) {
			value = row[columnIndex]
		}
		if _, held := counts[value]; !held {
			order = append(order, value)
		}
		counts[value]++
	}

	counted := make([]ValueCount, 0, len(order))
	for _, value := range order {
		counted = append(counted, ValueCount{Value: value, Count: counts[value]})
	}
	sort.SliceStable(counted, func(left, right int) bool {
		return counted[left].Count > counted[right].Count
	})
	return counted
}

// IsRowShown is true if the row matches every filtered column and contains the search term.
// A cell with a document shows a summary and not the text, so the search reads the document
// itself. Without this a search for a city inside an address would find nothing.
func IsRowShown(row []string, values []any, filter ScreenFilter) bool {
	for columnIndex, kept := range filter.Values {
		value := ""
		if columnIndex >= 0 && columnIndex < len(row) {
			value = row[columnIndex]
		}
		if !kept[value] {
			return false
		}
	}
	if filter.Search == "" {
		return true
	}
	term := strings.ToLower(filter.Search)
	for _, cell := range row {
		if strings.Contains(strings.ToLower(cell), term) {
			return true
		}
	}
	for _, value := range values {
		held, isDocument := value.(core.DocumentValue)
		if isDocument && strings.Contains(strings.ToLower(held.Text), term) {
			return true
		}
	}
	return false
}

// ApplyValueFilter sets the selected values of one column. A selection of every value, or of
// no value, removes the entry.
func ApplyValueFilter(
	filter ScreenFilter, columnIndex int, kept map[string]bool, available int,
) ScreenFilter {
	values := map[int]map[string]bool{}
	maps.Copy(values, filter.Values)
	if len(kept) == 0 || len(kept) >= available {
		delete(values, columnIndex)
	} else {
		copied := map[string]bool{}
		for value := range kept {
			copied[value] = true
		}
		values[columnIndex] = copied
	}
	return ScreenFilter{Values: values, Search: filter.Search}
}

// ApplySearchTerm sets the term the user typed. An empty term clears the search.
func ApplySearchTerm(filter ScreenFilter, term string) ScreenFilter {
	return ScreenFilter{Values: filter.Values, Search: strings.TrimSpace(term)}
}

// fnvPrime64 is the prime of FNV-1a. It mixes one index into the hash. The library hashes a
// text, and a number is mixed in here.
const fnvPrime64 = 1099511628211

// Fingerprint identifies the filter exactly, for a caller that caches the result of one. The
// banner is a summary and two different filters can have the same banner, so this hashes every
// value and not the number of values.
func (filter ScreenFilter) Fingerprint() uint64 {
	held := fnv.New64a()
	_, _ = held.Write([]byte(filter.Search))
	running := held.Sum64()

	indexes := make([]int, 0, len(filter.Values))
	for index := range filter.Values {
		indexes = append(indexes, index)
	}
	slices.Sort(indexes)

	for _, index := range indexes {
		running = running*fnvPrime64 + uint64(index) + 1
		// The kept values are a set, so each one is added and the order is not used.
		for value := range filter.Values[index] {
			one := fnv.New64a()
			_, _ = one.Write([]byte(value))
			running += one.Sum64()
		}
	}
	return running
}

// DescribeScreenFilter returns the filter in the form of the banner, so the user sees why a
// row is hidden.
func DescribeScreenFilter(filter ScreenFilter, columnNames []string) string {
	indexes := make([]int, 0, len(filter.Values))
	for index := range filter.Values {
		indexes = append(indexes, index)
	}
	slices.Sort(indexes)

	parts := make([]string, 0, len(indexes))
	for _, index := range indexes {
		name := fmt.Sprintf("column %d", index+1)
		if index >= 0 && index < len(columnNames) {
			name = columnNames[index]
		}
		kept := filter.Values[index]
		if len(kept) == 1 {
			for value := range kept {
				parts = append(parts, name+" = "+value)
			}
			continue
		}
		parts = append(parts, fmt.Sprintf("%s of %d", name, len(kept)))
	}

	written := []string{}
	if len(parts) > 0 {
		written = append(written, "showing "+strings.Join(parts, " · "))
	}
	if filter.Search != "" {
		written = append(written, "rows matching "+filter.Search)
	}
	return strings.Join(written, " · ")
}
