package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// The glyphs that say what a thing is come from the icon set, so `[ui] icons` chooses which
// ones are drawn and `[ui.icon_glyphs]` writes any one of them, or writes it as nothing to
// turn that one kind off. A glyph drawn from a constant of its own would answer neither.

// buildIconModel answers a model drawn with this icon set, on a catalog and a result.
func buildIconModel(t *testing.T, set cfg.IconSetName, glyphs map[cfg.IconKind]string) *Model {
	t.Helper()
	model := buildLoadedModel(t, 2, 5, 20, 4)
	model.icons = BuildIconSet(set, glyphs)
	connection := model.Active()
	connection.Session.(*offlineSession).capabilities = core.Capabilities{
		SortsRead: true, WritesDDL: true, TruncatesTable: true,
	}
	connection.Tree.Expanded[core.BuildSchemaID("schema_00")] = true
	connection.Autocommit = false
	connection.Active().Focus = app.PaneResult
	return model
}

// buildObjectMenuOn opens the object menu of a table on this model.
func buildObjectMenuOn(model *Model) {
	connection := model.Active()
	node := present.TreeNode{Kind: present.NodeTable, Table: db.TableRef{
		Schema: "schema_00", Name: "table_0000", Kind: db.RelationTable}}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayObjectMenu, Title: app.BuildObjectTitle(node),
		Draft:   app.NewEditorBuffer("", 0),
		Actions: app.BuildObjectActions(node, connection.Session.Capabilities()),
	}
}

// findRowHolding answers the first row of the frame that holds this text inside these cells.
// The panes draw beside one another, so the cells say which pane the row is read from.
func findRowHolding(frame []string, held string, from, to int) (string, bool) {
	for _, line := range frame {
		cells := readRowCells(line)
		if to >= len(cells) {
			to = len(cells) - 1
		}
		if from > to {
			continue
		}
		written := strings.Join(cells[from:to+1], "")
		if strings.Contains(written, held) {
			return strings.TrimRight(written, " "), true
		}
	}
	return "", false
}

// checkRowDrawsNoGlyph fails where the row holds a glyph of the plain set, or the blank one
// would have stood in. A set that draws nothing must leave no gap where a glyph was.
func checkRowDrawsNoGlyph(t *testing.T, name, row string, kinds ...cfg.IconKind) {
	t.Helper()
	plain := BuildIconSet(cfg.IconsPlain, nil)
	for _, kind := range kinds {
		if glyph := plain.Icon(kind); glyph != "" && strings.Contains(row, glyph) {
			t.Errorf("%s still draws the glyph %q of %q: %q", name, glyph, kind, row)
		}
	}
	if strings.Contains(strings.TrimLeft(row, " "), "  ") &&
		!strings.Contains(row, "   ") {
		t.Errorf("%s left a gap where a glyph stood: %q", name, row)
	}
}

// blankGlyphs answers the kinds of a frame written as nothing, which is how a config file
// turns one off.
func blankGlyphs(kinds ...cfg.IconKind) map[cfg.IconKind]string {
	written := map[cfg.IconKind]string{}
	for _, kind := range kinds {
		written[kind] = ""
	}
	return written
}

// A glyph written as nothing draws nothing, and leaves no gap where it stood. A column a menu
// keeps for a glyph and the blank a chip keeps for one both come off with it.
func TestAGlyphWrittenAsNothingLeavesNoGap(t *testing.T) {
	off := blankGlyphs(cfg.IconKinds...)
	model := buildIconModel(t, cfg.IconsPlain, off)
	frame := strings.Split(model.render(), "\n")

	tree := model.layout.treeRows
	for _, held := range []struct {
		name     string
		holds    string
		from, to int
		kinds    []cfg.IconKind
	}{
		{"a row of the tree", "table_0001", tree.from, tree.to,
			[]cfg.IconKind{cfg.IconTable}},
		{"a row of a schema", "schema_00", tree.from, tree.to,
			[]cfg.IconKind{cfg.IconSchema}},
		{"the strip of views", " data ", 0, model.width - 1,
			[]cfg.IconKind{cfg.IconTable, cfg.IconColumn}},
		{"the title bar", "masume", 0, model.width - 1,
			[]cfg.IconKind{cfg.IconAi}},
	} {
		row, found := findRowHolding(frame, held.holds, held.from, held.to)
		if !found {
			t.Errorf("no row of the frame holds %q", held.holds)
			continue
		}
		checkRowDrawsNoGlyph(t, held.name, row, held.kinds...)
	}

	buildObjectMenuOn(model)
	menu := strings.Split(model.render(), "\n")
	card := model.layout.overlayRows
	row, found := findRowHolding(menu, "ER diagram", card.from, card.to)
	if !found {
		t.Fatal("the object menu drew no row")
	}
	checkRowDrawsNoGlyph(t, "a row of the object menu", row,
		cfg.IconForeignKey, cfg.IconQuery, cfg.IconNote)
}

// A glyph set in the config is what the client draws, whichever set it started from.
func TestAGlyphSetInTheConfigIsDrawn(t *testing.T) {
	for _, set := range cfg.IconSetNames {
		t.Run(string(set), func(t *testing.T) {
			model := buildIconModel(t, set, map[cfg.IconKind]string{cfg.IconTable: "T"})
			frame := strings.Split(model.render(), "\n")
			tree := model.layout.treeRows

			row, found := findRowHolding(frame, "table_0001", tree.from, tree.to)
			if !found {
				t.Fatal("the tree drew no row for the relation")
			}
			if !strings.Contains(row, "T table_0001") {
				t.Errorf("the row reads %q and does not carry the glyph set for it", row)
			}
		})
	}
}

// Every kind the config file may name has a glyph in the set the client opens with, so a kind
// added to the client is never drawn as nothing by accident.
func TestEveryKindHasAGlyphInTheSetTheClientOpensWith(t *testing.T) {
	plain := BuildIconSet(cfg.IconsPlain, nil)
	for _, kind := range cfg.IconKinds {
		if plain.Icon(kind) == "" {
			t.Errorf("the kind %q has no glyph in the plain set", kind)
		}
	}
}

// A kind the plain set draws with a glyph a terminal may not have needs one in the set of
// letters as well, so a terminal without the font still draws every mark.
func TestTheSetOfLettersCoversTheKindsItCan(t *testing.T) {
	letters := BuildIconSet(cfg.IconsASCII, nil)
	for _, kind := range cfg.IconKinds {
		if letters.Icon(kind) == "" {
			t.Errorf("the kind %q has no letter to stand for it", kind)
		}
	}
}
