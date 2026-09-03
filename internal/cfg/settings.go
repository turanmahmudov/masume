package cfg

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// IconKind is the kind of one tree row. The config file uses these words.
type IconKind string

// The kinds a tree row can have.
const (
	IconSchema           IconKind = "schema"
	IconTable            IconKind = "table"
	IconView             IconKind = "view"
	IconMaterializedView IconKind = "materialized-view"
	IconFunction         IconKind = "function"
	IconSequence         IconKind = "sequence"
	IconType             IconKind = "type"
	IconTrigger          IconKind = "trigger"
	IconColumn           IconKind = "column"
	IconIndex            IconKind = "index"
	IconPlan             IconKind = "plan"
	IconPrimaryKey       IconKind = "primary-key"
	IconForeignKey       IconKind = "foreign-key"
	IconRole             IconKind = "role"
	IconRoles            IconKind = "roles"
	IconFavourites       IconKind = "favourites"
	IconRecent           IconKind = "recent"
	IconQuery            IconKind = "query"
	IconFolder           IconKind = "folder"
	IconNote             IconKind = "note"
	IconAi               IconKind = "ai"
	IconProblem          IconKind = "problem"

	// The icons of a control and not of a kind of object: the action of a row, the
	// result of a key press on it, and the state the client waits for.
	IconFoldClosed IconKind = "fold-closed"
	IconFoldOpen   IconKind = "fold-open"
	IconField      IconKind = "field"
	IconClose      IconKind = "close"
	IconDot        IconKind = "dot"
	IconSortUp     IconKind = "sort-up"
	IconSortDown   IconKind = "sort-down"
	IconPrompt     IconKind = "prompt"
	IconStepBack   IconKind = "step-back"
	IconStepOn     IconKind = "step-on"
	IconBanner     IconKind = "banner"
	IconNewTab     IconKind = "new-tab"
)

// IconKinds lists every kind a config file can set a glyph for.
var IconKinds = []IconKind{
	IconSchema, IconTable, IconView, IconMaterializedView, IconFunction, IconSequence,
	IconType, IconTrigger, IconColumn, IconIndex, IconPlan, IconPrimaryKey, IconForeignKey,
	IconRole, IconRoles, IconFavourites, IconRecent, IconQuery, IconFolder, IconNote, IconAi,
	IconProblem,
	IconFoldClosed, IconFoldOpen, IconField, IconClose, IconDot, IconSortUp, IconSortDown,
	IconPrompt, IconStepBack, IconStepOn, IconBanner, IconNewTab,
}

// IsIconKind is true if the text is a kind name.
func IsIconKind(written string) bool {
	for _, kind := range IconKinds {
		if string(kind) == written {
			return true
		}
	}
	return false
}

// IconSetName is the name of the glyph set the tree draws.
type IconSetName string

// The two sets included in the app. A set selects which glyphs are drawn, not whether
// glyphs are drawn at all: an empty glyph in `[ui.icon_glyphs]` disables one kind.
const (
	IconsPlain IconSetName = "plain"
	IconsASCII IconSetName = "ascii"
)

// IconSetNames lists the sets a config file can use.
var IconSetNames = []IconSetName{IconsPlain, IconsASCII}

// DescribeIconSetNames returns the valid set names, so an error message can list them.
func DescribeIconSetNames() string {
	written := make([]string, 0, len(IconSetNames))
	for _, name := range IconSetNames {
		written = append(written, string(name))
	}
	return "The sets are " + strings.Join(written, " and ") +
		". A glyph written as nothing turns one kind off."
}

// FindIconSetName parses the text as a set name.
func FindIconSetName(written string) (IconSetName, bool) {
	return core.FindAllowed(IconSetNames, written)
}

// UISettings holds everything under `[ui]`. These settings belong to the app and not to a
// profile.
type UISettings struct {
	IconSet IconSetName
	// A glyph the user selected for one kind, for example a Nerd Font glyph.
	IconGlyphs        map[IconKind]string
	HideSystemSchemas bool
	// The name of the colour theme, or empty for the default theme.
	Theme string
	// Colours set here and not in a theme file. They are applied over the selected theme.
	Colors ThemeTables
	// The colour settings under `[ui]` that could not be read.
	ColorProblems []string
	// The settings under `[ui]` that name something the client does not have, for
	// example an icon kind or an icon set. An unknown name is reported and not ignored,
	// so the user sees a spelling error.
	Problems []string
}

// DefaultUISettings holds the settings the app starts with.
func DefaultUISettings() UISettings {
	return UISettings{
		IconSet:           IconsPlain,
		IconGlyphs:        map[IconKind]string{},
		HideSystemSchemas: true,
		Colors:            NewThemeTables(),
	}
}

// ParseUISettings reads `[ui]`. An invalid setting uses the default, so a spelling error
// does not stop the app.
func ParseUISettings(document Table) UISettings {
	settings := DefaultUISettings()
	ui, present := FindSection(document, "ui")
	if !present {
		return settings
	}

	if written, named := FindString(ui, "icons"); named {
		if set, known := FindIconSetName(written); known {
			settings.IconSet = set
		} else {
			settings.Problems = append(settings.Problems,
				"icons: \""+written+"\" is not a set there is, so "+
					string(settings.IconSet)+" is drawn. "+DescribeIconSetNames())
		}
	}

	if glyphs, isTable := FindTable(ui["icon_glyphs"]); isTable {
		for _, kind := range sortedKeys(glyphs) {
			written, isText := glyphs[kind].(string)
			if !isText {
				continue
			}
			if !IsIconKind(kind) {
				settings.Problems = append(settings.Problems,
					"icon_glyphs: \""+kind+"\" is not a kind there is")
				continue
			}
			settings.IconGlyphs[IconKind(kind)] = written
		}
	}

	if hidden, isFlag := FindBool(ui, "hide_system_schemas"); isFlag {
		settings.HideSystemSchemas = hidden
	}
	settings.Theme, _ = FindString(ui, "theme")

	tables, problems := ParseThemeTables(ui)
	settings.Colors = tables
	settings.ColorProblems = problems
	return settings
}
