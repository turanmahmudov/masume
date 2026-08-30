package ui

import (
	"embed"
	"fmt"
	"image/color"
	"maps"
	"path"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// Turns a theme file into a theme. A file names colours, points one name at another, styles
// a highlight and inherits the rest. Nothing here draws.

//go:embed themes/*.toml
var shippedThemes embed.FS

// FallbackThemeName is the theme used at the start, and for every key a document leaves out.
const FallbackThemeName = "ayu-dark"

// shippedThemeOrder is the order the picker lists the themes the app ships in.
var shippedThemeOrder = []string{
	"ayu-dark", "tokyonight", "catppuccin-mocha", "gruvbox-dark", "dracula", "nord",
	"rose-pine", "solarized-dark", "catppuccin-latte", "gruvbox-light", "rose-pine-dawn",
}

// SystemThemeName is derived from the terminal, so no file may use this name.
const SystemThemeName = "system"

// systemThemeTitle is the title of the terminal theme, as a person reads it.
const systemThemeTitle = "System"

// chainLimit is the longest chain of names allowed before it counts as a loop.
const chainLimit = 8

// missingColor is drawn where a theme names no colour, so the gap is easy to see.
const missingColor = "#ff00ff"

// ThemeRegistry holds every theme document read, and the colours `[ui]` lays over them.
type ThemeRegistry struct {
	documents map[string]cfg.ThemeDocument
	// The order the themes were registered in, so the picker lists them the same way.
	order     []string
	overrides cfg.ThemeTables
	// The faults found in the themes the app ships. These are faults in the app.
	builtInProblems []string
	// The colours the terminal last reported, which the system theme is built from.
	terminal TerminalColors
}

// NewThemeRegistry reads the themes the app ships. They load first, so the first frame has
// a theme before the config is read.
func NewThemeRegistry() *ThemeRegistry {
	registry := &ThemeRegistry{
		documents: map[string]cfg.ThemeDocument{},
		overrides: cfg.NewThemeTables(),
	}

	entries, err := shippedThemes.ReadDir("themes")
	if err != nil {
		registry.builtInProblems = append(registry.builtInProblems,
			fmt.Sprintf("the themes the app ships did not read: %v", err))
		return registry
	}

	// The picker lists the dark themes first and ayu-dark before them all, because every
	// other theme falls back on it. A file the list does not name follows in name order.
	named := map[string]bool{}
	for _, name := range shippedThemeOrder {
		named[name] = true
	}
	rest := []string{}
	for _, entry := range entries {
		if held := strings.TrimSuffix(entry.Name(), ".toml"); !named[held] {
			rest = append(rest, held)
		}
	}
	slices.Sort(rest)
	names := append(append([]string{}, shippedThemeOrder...), rest...)

	for _, themeName := range names {
		name := themeName + ".toml"
		text, readErr := shippedThemes.ReadFile(path.Join("themes", name))
		if readErr != nil {
			registry.builtInProblems = append(registry.builtInProblems,
				fmt.Sprintf("theme %q: %v", themeName, readErr))
			continue
		}
		document, decodeErr := cfg.DecodeDocument(string(text))
		if decodeErr != nil {
			registry.builtInProblems = append(registry.builtInProblems,
				fmt.Sprintf("theme %q: %v", themeName, decodeErr))
			continue
		}
		parsed, problems := cfg.ParseThemeDocument(document, themeName)
		registry.keep(parsed)
		for _, problem := range problems {
			registry.builtInProblems = append(registry.builtInProblems,
				fmt.Sprintf("theme %q: %s", themeName, problem))
		}
	}
	return registry
}

func (registry *ThemeRegistry) keep(document cfg.ThemeDocument) {
	if _, held := registry.documents[document.Name]; !held {
		registry.order = append(registry.order, document.Name)
	}
	registry.documents[document.Name] = document
}

// ListBuiltInProblems returns the faults in a theme the app ships.
func (registry *ThemeRegistry) ListBuiltInProblems() []string {
	return registry.builtInProblems
}

// RegisterDocuments keeps the themes a user wrote. A file replaces a shipped theme of the
// same name, which is how a shipped theme is corrected in place.
func (registry *ThemeRegistry) RegisterDocuments(added []cfg.ThemeDocument) []string {
	problems := []string{}
	for _, document := range added {
		if document.Name == SystemThemeName {
			problems = append(problems, fmt.Sprintf(
				"theme %q follows the terminal and cannot be written", SystemThemeName))
			continue
		}
		registry.keep(document)
	}
	return problems
}

// ApplyOverrides keeps the colours from `[ui]`. They are laid over every theme from now on.
func (registry *ThemeRegistry) ApplyOverrides(tables cfg.ThemeTables) {
	registry.overrides = tables
}

// ListDocuments returns every theme document read.
func (registry *ThemeRegistry) ListDocuments() []cfg.ThemeDocument {
	listed := make([]cfg.ThemeDocument, 0, len(registry.order))
	for _, name := range registry.order {
		listed = append(listed, registry.documents[name])
	}
	return listed
}

// collectChain returns the chain of documents from the chosen theme to the last one it
// extends, and what the chain got wrong.
func (registry *ThemeRegistry) collectChain(name string) ([]cfg.ThemeDocument, []string) {
	chain := []cfg.ThemeDocument{}
	problems := []string{}
	seen := map[string]bool{}
	next := name

	for next != "" && len(chain) < chainLimit {
		if seen[next] {
			problems = append(problems, fmt.Sprintf(
				"theme %q extends itself, so the chain stops there", next))
			break
		}
		seen[next] = true
		document, known := registry.documents[next]
		if !known {
			problems = append(problems, fmt.Sprintf(
				"theme %q is not one of the themes there are", next))
			break
		}
		chain = append(chain, document)
		// A theme that extends nothing still falls back, so a short file is a whole theme.
		switch {
		case document.Extends != "":
			next = document.Extends
		case document.Name == FallbackThemeName:
			next = ""
		default:
			next = FallbackThemeName
		}
	}
	return chain, problems
}

// resolveAppearance returns the first theme of the chain that writes one, and dark where
// none of them does.
func resolveAppearance(chain []cfg.ThemeDocument) cfg.Appearance {
	for _, document := range chain {
		if document.Appearance != cfg.AppearanceUnset {
			return document.Appearance
		}
	}
	return cfg.AppearanceDark
}

// ResolveAppearance returns the appearance of the theme of that name, from the chain it
// extends.
func (registry *ThemeRegistry) ResolveAppearance(name string) cfg.Appearance {
	if name == SystemThemeName {
		return registry.systemAppearance()
	}
	chain, _ := registry.collectChain(name)
	return resolveAppearance(chain)
}

func mergeTables(over, under cfg.ThemeTables) cfg.ThemeTables {
	merged := cfg.NewThemeTables()
	maps.Copy(merged.Palette, under.Palette)
	maps.Copy(merged.Palette, over.Palette)
	maps.Copy(merged.Colors, under.Colors)
	maps.Copy(merged.Colors, over.Colors)
	maps.Copy(merged.Syntax, under.Syntax)
	for kind, rule := range over.Syntax {
		// A link replaces the highlight, and does not add to the inherited one.
		if rule.HasLink {
			merged.Syntax[kind] = rule
			continue
		}
		merged.Syntax[kind] = mergeSyntaxRule(merged.Syntax[kind], rule)
	}
	return merged
}

func mergeSyntaxRule(under, over cfg.SyntaxRule) cfg.SyntaxRule {
	merged := under
	if over.HasForeground {
		merged.Foreground, merged.HasForeground = over.Foreground, true
	}
	if over.HasBackground {
		merged.Background, merged.HasBackground = over.Background, true
	}
	if over.HasBold {
		merged.Bold, merged.HasBold = over.Bold, true
	}
	if over.HasItalic {
		merged.Italic, merged.HasItalic = over.Italic, true
	}
	if over.HasUnderline {
		merged.Underline, merged.HasUnderline = over.Underline, true
	}
	if over.HasLink {
		merged.Link, merged.HasLink = over.Link, true
	}
	return merged
}

// mergedTheme is a theme after its chain is merged into one document.
type mergedTheme struct {
	cfg.ThemeTables
	name       string
	title      string
	appearance cfg.Appearance
}

// mergeChain merges the chain from the far end forward, so the chosen theme wins.
func (registry *ThemeRegistry) mergeChain(chain []cfg.ThemeDocument) (mergedTheme, bool) {
	if len(chain) == 0 {
		return mergedTheme{}, false
	}
	nearest := chain[0]

	tables := cfg.NewThemeTables()
	for _, c := range slices.Backward(chain) {
		tables = mergeTables(c.ThemeTables, tables)
	}

	return mergedTheme{
		ThemeTables: mergeTables(registry.overrides, tables),
		name:        nearest.Name,
		title:       nearest.Title,
		appearance:  resolveAppearance(chain),
	}, true
}

// resolveColorValue returns a colour a theme wrote. A hex value is itself. Anything else is
// a name: a palette entry, or another colour of the theme, as in `border_focus = "accent"`.
func resolveColorValue(written string, tables cfg.ThemeTables, seen map[string]bool) string {
	if cfg.IsHexColor(written) {
		return written
	}
	if fromPalette, held := tables.Palette[written]; held {
		return fromPalette
	}
	fromColors, held := tables.Colors[written]
	if !held || seen[written] || len(seen) >= chainLimit {
		return ""
	}
	seen[written] = true
	return resolveColorValue(fromColors, tables, seen)
}

// resolveWrittenColors reads every colour of the theme, and reports the ones it could not.
func resolveWrittenColors(merged mergedTheme) (colorsDraft, []string) {
	draft := colorsDraft{}
	problems := []string{}
	held := map[string]bool{}

	for _, entry := range writtenColors {
		written, present := merged.Colors[entry.written]
		if !present {
			if !derivedColors[entry.written] {
				problems = append(problems, fmt.Sprintf(
					"theme %q names no %s", merged.name, entry.written))
				*entry.read(&draft.ThemeColors) = lipgloss.Color(missingColor)
			}
			continue
		}
		resolved := resolveColorValue(written, merged.ThemeTables, map[string]bool{})
		if resolved == "" {
			problems = append(problems, fmt.Sprintf(
				"%s of theme %q is %q, which is neither a hex colour nor a name it gives",
				entry.written, merged.name, written))
			resolved = missingColor
		}
		*entry.read(&draft.ThemeColors) = lipgloss.Color(resolved)
		held[entry.written] = true
	}

	draft.hasOnAccent = held["on_accent"]
	draft.hasSelection = held["selection"]
	draft.hasEnvDev = held["env_dev"]
	draft.hasEnvTest = held["env_test"]
	draft.hasEnvProd = held["env_prod"]
	return draft, problems
}

// buildSyntaxLookup returns how a name in `[syntax]` is read. It reaches every colour of
// the theme, derived ones included.
func buildSyntaxLookup(
	tables cfg.ThemeTables, colors ThemeColors,
) func(string) (color.Color, bool) {
	byName := map[string]color.Color{}
	for _, entry := range writtenColors {
		held := colors
		byName[entry.written] = *entry.read(&held)
	}
	return func(written string) (color.Color, bool) {
		if cfg.IsHexColor(written) {
			return lipgloss.Color(written), true
		}
		if fromPalette, held := tables.Palette[written]; held {
			return lipgloss.Color(fromPalette), true
		}
		if named, held := byName[written]; held && named != nil {
			return named, true
		}
		return nil, false
	}
}

// flattenRule returns one highlight rule, with the rule it links to underneath it, and what
// the links got wrong.
func flattenRule(
	kind string, syntax map[string]cfg.SyntaxRule, seen map[string]bool,
) (cfg.SyntaxRule, []string) {
	rule := syntax[kind]
	if !rule.HasLink {
		return rule, nil
	}
	if seen[rule.Link] || len(seen) >= chainLimit {
		problem := fmt.Sprintf(
			"highlight %q links back to itself, so the link is dropped", kind)
		rule.Link, rule.HasLink = "", false
		return rule, []string{problem}
	}
	seen[kind] = true
	own := rule
	own.Link, own.HasLink = "", false
	linked, problems := flattenRule(rule.Link, syntax, seen)
	return mergeSyntaxRule(linked, own), problems
}

func resolveHighlight(
	kind HighlightKind, syntax map[string]cfg.SyntaxRule,
	lookUp func(string) (color.Color, bool),
) (HighlightStyle, []string) {
	rule, problems := flattenRule(string(kind), syntax, map[string]bool{})
	readColor := func(written string, present bool, part string) (color.Color, bool) {
		if !present {
			return nil, false
		}
		resolved, known := lookUp(written)
		if known {
			return resolved, true
		}
		problems = append(problems, fmt.Sprintf(
			"the %s of highlight %q is %q, which names no colour", part, kind, written))
		return nil, false
	}

	style := HighlightStyle{Bold: rule.Bold, Italic: rule.Italic, Underline: rule.Underline}
	style.Foreground, style.HasForeground = readColor(rule.Foreground, rule.HasForeground, "colour")
	style.Background, style.HasBackground = readColor(rule.Background, rule.HasBackground, "ground")
	return style, problems
}

// ResolvedTheme is a theme ready to draw with, and the problems in its files.
type ResolvedTheme struct {
	Theme    Theme
	Problems []string
}

// buildTheme turns a merged document into a theme, and reports what the document got wrong.
// `kept` holds a colour the caller resolved itself.
func buildTheme(merged mergedTheme, kept func(*ThemeColors)) (Theme, []string) {
	draft, problems := resolveWrittenColors(merged)
	if kept != nil {
		kept(&draft.ThemeColors)
	}
	colors := buildThemeColors(draft)

	syntax := map[HighlightKind]HighlightStyle{}
	lookUp := buildSyntaxLookup(merged.ThemeTables, colors)
	for _, kind := range HighlightKinds {
		style, faults := resolveHighlight(kind, merged.Syntax, lookUp)
		syntax[kind] = style
		problems = append(problems, faults...)
	}

	return Theme{
		ThemeColors: colors, Name: merged.name, Title: merged.title,
		Appearance: merged.appearance, Syntax: syntax,
	}, problems
}

// FindResolvedTheme returns the theme of that name, with what it inherits and what `[ui]`
// changed. It returns nothing where there is no theme of that name.
func (registry *ThemeRegistry) FindResolvedTheme(name string) (ResolvedTheme, bool) {
	if name == SystemThemeName {
		return registry.ResolveSystemTheme(), true
	}
	chain, problems := registry.collectChain(name)
	merged, found := registry.mergeChain(chain)
	if !found {
		return ResolvedTheme{}, false
	}
	theme, faults := buildTheme(merged, nil)
	return ResolvedTheme{Theme: theme, Problems: append(problems, faults...)}, true
}

// ThemeChoice is one row per theme, for the list to draw.
type ThemeChoice struct {
	Name       string
	Title      string
	Appearance cfg.Appearance
}

// ListThemeChoices returns every theme the picker offers, with the terminal one last.
func (registry *ThemeRegistry) ListThemeChoices() []ThemeChoice {
	choices := make([]ThemeChoice, 0, len(registry.order)+1)
	for _, document := range registry.ListDocuments() {
		choices = append(choices, ThemeChoice{
			Name: document.Name, Title: document.Title,
			Appearance: registry.ResolveAppearance(document.Name),
		})
	}
	return append(choices, ThemeChoice{
		Name: SystemThemeName, Title: systemThemeTitle,
		Appearance: registry.systemAppearance(),
	})
}
