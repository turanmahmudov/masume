package present

import (
	"fmt"
	"hash/fnv"
	"maps"
	"slices"
	"sort"
	"strings"
)

// A filter over the rows on screen. It hides rows and reads none, so its counts are of the
// page, not of the relation. A WHERE narrows the whole relation.

// ValueCount is one value of the column, and how many rows on screen have it.
type ValueCount struct {
	Value string
	Count int
}

// ScreenFilter is the filter of the grid: the values kept per column, and a term over
// every column.
type ScreenFilter struct {
	// The values kept per column. A column without an entry is not filtered.
	Values map[int]map[string]bool
	Search string
}

// NoScreenFilter hides nothing.
func NoScreenFilter() ScreenFilter {
	return ScreenFilter{Values: map[int]map[string]bool{}}
}

// IsEmpty is true where the filter hides nothing.
func (filter ScreenFilter) IsEmpty() bool {
	return len(filter.Values) == 0 && filter.Search == ""
}

// CountColumnValues returns the values of one column, the most common first, then in the
// order they were found.
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

// IsRowShown is true if the row matches every filtered column and holds the term somewhere.
func IsRowShown(row []string, filter ScreenFilter) bool {
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
	return false
}

// ApplyValueFilter keeps the chosen values of one column. Keeping every value, or none,
// clears the entry.
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

// ApplySearchTerm keeps the term the user typed. An empty term clears the search.
func ApplySearchTerm(filter ScreenFilter, term string) ScreenFilter {
	return ScreenFilter{Values: filter.Values, Search: strings.TrimSpace(term)}
}

// fnvPrime64 is the prime of FNV-1a, which mixes one index into the running hash. The
// library hashes a text, and a number is mixed in by hand.
const fnvPrime64 = 1099511628211

// Fingerprint identifies the filter exactly, for a caller that keeps what it drew from one.
// The banner is a summary and two different filters can read alike in it, so this reads every
// value rather than counting them.
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
		// The kept values are a set, so each one is added rather than written in turn.
		for value := range filter.Values[index] {
			one := fnv.New64a()
			_, _ = one.Write([]byte(value))
			running += one.Sum64()
		}
	}
	return running
}

// DescribeScreenFilter writes the filter as the banner shows it, so a hidden row is
// explained.
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
