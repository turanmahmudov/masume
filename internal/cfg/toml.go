// Package cfg reads the config file: the connection profiles, the interface settings, the
// colour themes and the keys.
package cfg

import "strings"

// Table is a parsed TOML table.
type Table map[string]any

// FindTable returns the value as a table, and false for a list, a scalar or a missing
// key.
func FindTable(value any) (Table, bool) {
	switch held := value.(type) {
	case Table:
		return held, true
	case map[string]any:
		return Table(held), true
	}
	return nil, false
}

// FindSection returns a top-level section of the document, for example `[ai]`.
func FindSection(document Table, name string) (Table, bool) {
	if document == nil {
		return nil, false
	}
	return FindTable(document[name])
}

// FindString returns the text of a key. An empty text is treated as a missing key.
func FindString(table Table, key string) (string, bool) {
	written, isText := table[key].(string)
	if !isText || strings.TrimSpace(written) == "" {
		return "", false
	}
	return written, true
}

// FindInteger returns the integer of a key.
func FindInteger(table Table, key string) (int, bool) {
	switch held := table[key].(type) {
	case int64:
		return int(held), true
	case int:
		return held, true
	case float64:
		if held == float64(int64(held)) {
			return int(held), true
		}
	}
	return 0, false
}

// FindPositiveInteger returns the integer of a key, and false if it is not above zero.
func FindPositiveInteger(table Table, key string) (int, bool) {
	value, isWhole := FindInteger(table, key)
	if !isWhole || value <= 0 {
		return 0, false
	}
	return value, true
}

// FindBool returns the boolean of a key.
func FindBool(table Table, key string) (bool, bool) {
	held, isFlag := table[key].(bool)
	return held, isFlag
}

// FindStringList returns the strings of a list and skips every entry that is not a
// string.
func FindStringList(table Table, key string) ([]string, bool) {
	entries, isList := table[key].([]any)
	if !isList {
		return nil, false
	}
	texts := make([]string, 0, len(entries))
	for _, entry := range entries {
		written, isText := entry.(string)
		if isText && strings.TrimSpace(written) != "" {
			texts = append(texts, written)
		}
	}
	return texts, true
}
