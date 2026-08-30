package ui

import (
	"image/color"
	"maps"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/query/editor"
)

// IconSet holds one glyph per object kind.
type IconSet map[cfg.IconKind]string

// plainIcons draw every glyph one column wide in a terminal that draws ambiguous characters
// narrow.
var plainIcons = IconSet{
	cfg.IconSchema: "◇", cfg.IconTable: "▦", cfg.IconView: "◈",
	cfg.IconMaterializedView: "◆", cfg.IconFunction: "ƒ", cfg.IconSequence: "№",
	cfg.IconType: "⊞", cfg.IconTrigger: "⚑", cfg.IconColumn: "·",
	cfg.IconIndex: "▤", cfg.IconPlan: "⊳",
	cfg.IconPrimaryKey: "◆", cfg.IconForeignKey: "→", cfg.IconRole: "●",
	cfg.IconRoles: "●", cfg.IconFavourites: "★", cfg.IconRecent: "↻",
	cfg.IconQuery: "≡", cfg.IconFolder: "▸", cfg.IconNote: "⚠", cfg.IconAi: "✦",
	cfg.IconProblem:    "✗",
	cfg.IconFoldClosed: "▸", cfg.IconFoldOpen: "▾", cfg.IconField: "▸",
	cfg.IconClose: "×", cfg.IconDot: "●", cfg.IconSortUp: "↑", cfg.IconSortDown: "↓",
	cfg.IconPrompt: "❯", cfg.IconStepBack: "‹", cfg.IconStepOn: "›",
	cfg.IconBanner: "⚑", cfg.IconNewTab: "+",
}

// asciiIcons are for a terminal or a font that draws the glyphs above as empty boxes.
var asciiIcons = IconSet{
	cfg.IconSchema: "~", cfg.IconTable: "T", cfg.IconView: "V",
	cfg.IconMaterializedView: "M", cfg.IconFunction: "f", cfg.IconSequence: "S",
	cfg.IconType: "Y", cfg.IconTrigger: "!", cfg.IconColumn: ".",
	cfg.IconIndex: "#", cfg.IconPlan: ">",
	cfg.IconPrimaryKey: "*", cfg.IconForeignKey: ">", cfg.IconRole: "o",
	cfg.IconRoles: "o", cfg.IconFavourites: "*", cfg.IconRecent: "@",
	cfg.IconQuery: "=", cfg.IconFolder: ">", cfg.IconNote: "!", cfg.IconAi: "*",
	cfg.IconProblem:    "x",
	cfg.IconFoldClosed: ">", cfg.IconFoldOpen: "v", cfg.IconField: ">",
	cfg.IconClose: "x", cfg.IconDot: "o", cfg.IconSortUp: "^", cfg.IconSortDown: "v",
	cfg.IconPrompt: ">", cfg.IconStepBack: "<", cfg.IconStepOn: ">",
	cfg.IconBanner: "!", cfg.IconNewTab: "+",
}

// BuildIconSet returns the glyphs of a set, with the ones the user chose over them. A Nerd
// Font glyph cannot be shipped, because without the font every row is an empty box, so a
// reader who has the font writes the glyphs they want in `[ui.icon_glyphs]`. A glyph written
// as nothing turns that one kind off.
func BuildIconSet(name cfg.IconSetName, chosen map[cfg.IconKind]string) IconSet {
	base := plainIcons
	if name == cfg.IconsASCII {
		base = asciiIcons
	}
	built := IconSet{}
	maps.Copy(built, base)
	maps.Copy(built, chosen)
	return built
}

// Icon returns the glyph of that kind.
func (icons IconSet) Icon(kind cfg.IconKind) string {
	return icons[kind]
}

// Prefix returns the glyph and the space after it, or nothing while icons are off.
func (icons IconSet) Prefix(kind cfg.IconKind) string {
	glyph := icons[kind]
	if glyph == "" {
		return ""
	}
	return glyph + " "
}

// IconColor returns the colour each kind of object is drawn in.
func (styles *Styles) IconColor(kind cfg.IconKind) color.Color {
	switch kind {
	case cfg.IconSchema, cfg.IconRole, cfg.IconRoles, cfg.IconQuery:
		return styles.Theme.AccentAlt
	case cfg.IconTable:
		return styles.Theme.Accent
	case cfg.IconView, cfg.IconMaterializedView, cfg.IconForeignKey, cfg.IconRecent, cfg.IconAi:
		return styles.Theme.Info
	case cfg.IconFunction:
		return styles.Theme.Success
	case cfg.IconSequence, cfg.IconPrimaryKey, cfg.IconFavourites:
		return styles.Theme.Warning
	case cfg.IconType:
		return styles.Theme.AccentWarm
	case cfg.IconIndex:
		return styles.Theme.AccentAlt
	case cfg.IconPlan:
		return styles.Theme.Success
	case cfg.IconTrigger:
		return styles.Theme.Danger
	case cfg.IconNote:
		return styles.Theme.Error
	case cfg.IconProblem, cfg.IconBanner:
		return styles.Theme.Error
	case cfg.IconFoldClosed, cfg.IconFoldOpen, cfg.IconField, cfg.IconPrompt,
		cfg.IconStepBack, cfg.IconStepOn, cfg.IconNewTab, cfg.IconSortUp, cfg.IconSortDown:
		return styles.Theme.Accent
	}
	// A close mark, a dot and every kind with no colour of its own stand quietly.
	return styles.Theme.Muted
}

// CompletionKindColor returns the colour of one suggestion. A keyword takes the colour
// of the editor, and a name from the catalog takes the colour of the tree, so both look
// the same everywhere.
func (styles *Styles) CompletionKindColor(kind editor.CompletionKind) color.Color {
	switch kind {
	case editor.CompleteKeyword:
		return styles.Theme.Accent
	case editor.CompleteSchema:
		return styles.IconColor(cfg.IconSchema)
	case editor.CompleteTable:
		return styles.IconColor(cfg.IconTable)
	case editor.CompleteFunction:
		return styles.IconColor(cfg.IconFunction)
	case editor.CompleteColumn:
		return styles.Theme.Info
	}
	return styles.Theme.Muted
}
