package cfg

import (
	"fmt"
	"regexp"
	"slices"
)

// SyntaxRule is one highlight rule of a theme. Every part is optional.
type SyntaxRule struct {
	Foreground    string
	HasForeground bool
	Background    string
	HasBackground bool
	Bold          bool
	HasBold       bool
	Italic        bool
	HasItalic     bool
	Underline     bool
	HasUnderline  bool
	// The inherited rule. Local settings override the inherited style.
	Link    string
	HasLink bool
}

// ThemeTables is the palette, colours, and syntax configuration for a theme or `[ui]`.
type ThemeTables struct {
	// The named colours of the theme, each one a hex value.
	Palette map[string]string
	// One entry per colour role, each one a hex value or a palette name.
	Colors map[string]string
	Syntax map[string]SyntaxRule
}

// NewThemeTables returns three empty tables.
func NewThemeTables() ThemeTables {
	return ThemeTables{
		Palette: map[string]string{},
		Colors:  map[string]string{},
		Syntax:  map[string]SyntaxRule{},
	}
}

// Appearance is the theme terminal background: dark or light.
type Appearance string

// The two appearances a theme can set.
const (
	AppearanceDark  Appearance = "dark"
	AppearanceLight Appearance = "light"
	// AppearanceUnset means the file sets none, so the parent theme provides it.
	AppearanceUnset Appearance = ""
)

// ThemeDocument is a parsed theme file, before the colours are resolved.
type ThemeDocument struct {
	ThemeTables
	Name string
	// The title shown to the user. It is the theme name if the file has no title.
	Title      string
	Appearance Appearance
	// The theme that provides every key this one does not set.
	Extends string
}

// hexColor matches a colour written as a hex value. All other text is a palette name.
var hexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// IsHexColor is true if the text is a hex colour and not a palette name.
func IsHexColor(value string) bool {
	return hexColor.MatchString(value)
}

// sortedKeys returns table keys in alphabetical order.
func sortedKeys(table Table) []string {
	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// readColorTable reads a table of colours. Only a string can be a hex value or a palette
// name.
func readColorTable(table Table, label string, problems *[]string) map[string]string {
	read := map[string]string{}
	for _, key := range sortedKeys(table) {
		written, isText := table[key].(string)
		if isText {
			read[key] = written
			continue
		}
		*problems = append(*problems, fmt.Sprintf("%s %q must be text", label, key))
	}
	return read
}

// readPalette reads hex colours. Palette references are invalid in palette entries.
func readPalette(table Table, problems *[]string) map[string]string {
	read := readColorTable(table, "palette entry", problems)
	for _, key := range sortedNames(read) {
		if IsHexColor(read[key]) {
			continue
		}
		*problems = append(*problems, fmt.Sprintf(
			"palette entry %q has an invalid hex colour: %q", key, read[key]))
		delete(read, key)
	}
	return read
}

func sortedNames(table map[string]string) []string {
	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func findFlag(table Table, key, label string, problems *[]string) (bool, bool) {
	value, present := table[key]
	if !present {
		return false, false
	}
	held, isFlag := value.(bool)
	if isFlag {
		return held, true
	}
	*problems = append(*problems, fmt.Sprintf("%s %q must be true or false", label, key))
	return false, false
}

func readSyntaxRule(table Table, label string, problems *[]string) SyntaxRule {
	rule := SyntaxRule{}
	rule.Foreground, rule.HasForeground = FindString(table, "fg")
	rule.Background, rule.HasBackground = FindString(table, "bg")
	rule.Link, rule.HasLink = FindString(table, "link")
	rule.Bold, rule.HasBold = findFlag(table, "bold", label, problems)
	rule.Italic, rule.HasItalic = findFlag(table, "italic", label, problems)
	rule.Underline, rule.HasUnderline = findFlag(table, "underline", label, problems)
	return rule
}

func readSyntax(table Table, problems *[]string) map[string]SyntaxRule {
	read := map[string]SyntaxRule{}
	for _, kind := range sortedKeys(table) {
		rule, isTable := FindTable(table[kind])
		if !isTable {
			*problems = append(*problems, fmt.Sprintf(
				"highlight %q must be a table", kind))
			continue
		}
		read[kind] = readSyntaxRule(rule, fmt.Sprintf("highlight %q", kind), problems)
	}
	return read
}

// readAppearance returns the appearance of the file, or AppearanceUnset so the caller can
// use the parent theme.
func readAppearance(root Table, problems *[]string) Appearance {
	written, present := FindString(root, "appearance")
	if !present {
		return AppearanceUnset
	}
	if written == string(AppearanceDark) || written == string(AppearanceLight) {
		return Appearance(written)
	}
	*problems = append(*problems, fmt.Sprintf(
		"invalid appearance %q; use \"dark\" or \"light\"", written))
	return AppearanceDark
}

func readTables(root Table, problems *[]string) ThemeTables {
	paletteTable, _ := FindTable(root["palette"])
	colorTable, _ := FindTable(root["colors"])
	syntaxTable, _ := FindTable(root["syntax"])
	return ThemeTables{
		Palette: readPalette(paletteTable, problems),
		Colors:  readColorTable(colorTable, "colour", problems),
		Syntax:  readSyntax(syntaxTable, problems),
	}
}

// ParseThemeDocument reads a theme document. The caller supplies the theme file name.
func ParseThemeDocument(document Table, name string) (ThemeDocument, []string) {
	problems := []string{}
	if document == nil {
		document = Table{}
	}

	title, hasTitle := FindString(document, "title")
	if !hasTitle {
		title = name
	}
	extends, _ := FindString(document, "extends")

	return ThemeDocument{
		ThemeTables: readTables(document, &problems),
		Name:        name,
		Title:       title,
		Appearance:  readAppearance(document, &problems),
		Extends:     extends,
	}, problems
}

// ParseThemeTables reads the same three tables from `[ui]`. They are applied over the
// selected theme.
func ParseThemeTables(section Table) (ThemeTables, []string) {
	problems := []string{}
	if section == nil {
		section = Table{}
	}
	return readTables(section, &problems), problems
}
