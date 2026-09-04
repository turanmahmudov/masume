package ui

import (
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// The widths a card of an overlay is drawn at.
const (
	widestOverlayCard    = 96
	narrowestOverlayCard = 40
)

// renderOverlayOver draws the overlay in the middle of the frame it covers.
func (model *Model) renderOverlayOver(
	connection *app.Connection, tab *app.Tab, frame []string, height int,
) []string {
	// A card records what it draws in the cells of its own block, because only this knows
	// where the block lands on the screen.
	placedBars := len(model.layout.scrollbars)
	placedKeys := len(model.layout.buttons)
	card := model.renderOverlay(connection, tab, height)
	if card == "" {
		model.layout.scrollbars = model.layout.scrollbars[:placedBars]
		model.layout.buttons = model.layout.buttons[:placedKeys]
		return frame
	}
	// The card is drawn over the panes, which keep standing around it, so the reader
	// still sees what the overlay is about.
	rows := strings.Split(card, "\n")
	// The card is centred over everything under the title bar, the bottom bar included,
	// so the odd row sits over the card and not under it.
	top := max(halfRoundedUp(model.height-1-len(rows)), 0)
	left := max(halfRoundedUp(model.width-measureStyledWidth(card)), 0)
	// The frame this card is placed on is the workspace, and the title bar is put over it
	// afterwards, so a row of the screen is one more than a row of the frame.
	screenTop := top + titleBarRows

	// The inside of the card is the block a drag over it stays in, and it stands over the
	// panes, so it is looked at first.
	model.layout.selectionBlocks = append([]blockRect{{
		fromX: left + 1, toX: left + measureStyledWidth(card) - 2,
		fromY: screenTop + 1, toY: screenTop + len(rows) - 2,
	}}, model.layout.selectionBlocks...)

	model.placeCardHits(left, screenTop, placedBars, placedKeys)
	// A card owns the pointer while it is open, so the keys and the bars the panes behind
	// it drew are dropped. They are covered by the card or stand beside it, and a press or
	// a mark on one of them belongs to a pane the reader is not in.
	model.layout.buttons = append([]buttonHit{}, model.layout.buttons[placedKeys:]...)
	model.layout.scrollbars = append([]scrollHit{}, model.layout.scrollbars[placedBars:]...)
	return placeOver(frame, card, left, top, model.styles.Theme.Background)
}

// placeCardHits carries every hit box a card recorded from the cells of the card onto the
// cells of the screen. A card is drawn before it is placed, so it counts from its own top
// left corner and this is where the corner is learnt.
func (model *Model) placeCardHits(left, top, placedBars, placedKeys int) {
	layout := &model.layout
	for at := placedBars; at < len(layout.scrollbars); at++ {
		layout.scrollbars[at].column += left
		layout.scrollbars[at].top += top
	}
	for at := placedKeys; at < len(layout.buttons); at++ {
		layout.buttons[at].row += top
		layout.buttons[at].from += left
		layout.buttons[at].to += left
		layout.buttons[at].keyTo += left
	}
	for at := range layout.overlayChips {
		layout.overlayChips[at].row += top
		layout.overlayChips[at].from += left
		layout.overlayChips[at].to += left
	}
	for at := range layout.formChoices {
		layout.formChoices[at].row += top
		layout.formChoices[at].back += left
		layout.formChoices[at].on += left
	}
	layout.cardBodyTop += top
	layout.cardBodyLeft += left
	for _, block := range []*rowsHit{&layout.overlayRows, &layout.formRows} {
		if block.count < 1 {
			continue
		}
		block.top += top
		block.from += left
		block.to += left
	}
}

// overlayShares name the share of the screen each card takes, where it takes a share
// rather than a fixed width. A card that lists statements is wide; one that asks a
// question is not.
var overlayShares = map[app.OverlayKind]int{
	app.OverlayPalette:     70,
	app.OverlayHistory:     92,
	app.OverlaySaved:       86,
	app.OverlayActivity:    92,
	app.OverlayDiagram:     92,
	app.OverlayChanges:     86,
	app.OverlayWritePlan:   86,
	app.OverlayCellEdit:    80,
	app.OverlayCell:        80,
	app.OverlayValueFilter: 60,
	app.OverlayParameters:  60,
	app.OverlayThemePicker: 52,
	app.OverlayAiChat:      80,
	app.OverlayAiChats:     88,
}

// overlayWidths name the cards that keep one width, whatever the screen is.
var overlayWidths = map[app.OverlayKind]int{
	app.OverlayHelp:       78,
	app.OverlayExport:     76,
	app.OverlayImport:     84,
	app.OverlayConfirm:    76,
	app.OverlayChoice:     76,
	app.OverlayPrompt:     76,
	app.OverlayMessage:    72,
	app.OverlayObjectMenu: 72,
	app.OverlayActionMenu: 72,
	app.OverlayCopyMenu:   64,
	app.OverlayRowDetail:  92,
}

// resolveOverlayWidth returns how wide this card draws. The screen is the limit,
// because a wider card would draw over the border of the pane under it.
func (model *Model) resolveOverlayWidth(kind app.OverlayKind) int {
	wanted := widestOverlayCard
	if share, held := overlayShares[kind]; held {
		wanted = model.width * share / 100
	}
	if fixed, held := overlayWidths[kind]; held {
		wanted = fixed
	}
	if wanted > model.width {
		wanted = model.width
	}
	if wanted < narrowestOverlayCard {
		wanted = narrowestOverlayCard
	}
	return wanted
}

// The largest share of the screen a card may take, and the height under which a card is
// too small to read as a card. A card that filters a list takes the largest height, or the
// list would grow and shrink while the user types.
const (
	widestOverlayHeightShare = 70
	narrowestOverlayRow      = 7
)

// overlayHeightShares name the share of the screen height each card takes.
var overlayHeightShares = map[app.OverlayKind]int{
	app.OverlayHelp:        widestOverlayHeightShare,
	app.OverlayPalette:     widestOverlayHeightShare,
	app.OverlayHistory:     widestOverlayHeightShare,
	app.OverlaySaved:       widestOverlayHeightShare,
	app.OverlayActivity:    widestOverlayHeightShare,
	app.OverlayThemePicker: widestOverlayHeightShare,
	app.OverlayObjectMenu:  60,
	app.OverlayActionMenu:  60,
	app.OverlayAiChat:      80,
	app.OverlayAiChats:     widestOverlayHeightShare,
}

// overlayHeightRows name the cards that keep one height, whatever the screen is.
var overlayHeightRows = map[app.OverlayKind]int{
	app.OverlayCopyMenu: 12,
	app.OverlayMessage:  12,
	app.OverlayConfirm:  16,
	app.OverlayChoice:   16,
}

// resolveOverlayHeight returns how tall one card draws: the share of the screen it is given,
// the height it keeps, or the rows of what it shows with the chrome and the keys around them.
// The largest share is the ceiling, and a row more is taken where the rows left over cannot
// be shared evenly.
func (model *Model) resolveOverlayHeight(
	kind app.OverlayKind, contentRows, hintRows int,
) int {
	ceiling := model.height * widestOverlayHeightShare / 100
	asked := ceiling
	switch share, held := overlayHeightShares[kind]; {
	case held:
		asked = model.height * share / 100
	default:
		if rows, fixed := overlayHeightRows[kind]; fixed {
			asked = rows
		} else if contentRows > 0 {
			asked = contentRows + present.CardChrome + hintRows
		}
	}
	if asked > ceiling {
		asked = ceiling
	}
	if asked < narrowestOverlayRow {
		asked = narrowestOverlayRow
	}
	if asked > model.height {
		asked = model.height
	}
	return present.AlignCardHeight(asked, model.height)
}

// countHintRows returns the rows the keys of a card take, with the blank row over them.
func countHintRows(keys string, width int) int {
	if keys == "" {
		return 0
	}
	return 1 + present.CountWrappedRows(keys, width-present.CardChrome)
}

// renderOverlay draws the overlay on top, whichever one is open.
func (model *Model) renderOverlay(
	connection *app.Connection, tab *app.Tab, height int,
) string {
	overlay := connection.Overlay
	width := model.resolveOverlayWidth(overlay.Kind)
	// The hits of the card drawn before are dropped, so a press never lands on a chip
	// another card left behind.
	model.layout.overlayRows = rowsHit{}
	model.layout.overlayChips = nil
	model.layout.formRows = rowsHit{}
	model.layout.formChoices = nil

	switch overlay.Kind {
	case app.OverlayHelp:
		return model.renderHelp(overlay, width)
	case app.OverlayPalette:
		return model.renderPalette(overlay, width)
	case app.OverlayHistory:
		return model.renderHistory(overlay, width)
	case app.OverlaySaved:
		return model.renderSaved(overlay, width)
	case app.OverlayObjectMenu, app.OverlayCopyMenu, app.OverlayActionMenu:
		return model.renderMenu(overlay, width)
	case app.OverlayConfirm:
		return model.renderConfirm(overlay, width)
	case app.OverlayWritePlan:
		return model.renderWritePlan(overlay, width)
	case app.OverlayMessage:
		return model.renderMessage(overlay, width)
	case app.OverlayDiagram:
		return model.renderDiagram(overlay, width)
	case app.OverlayCell:
		return model.renderCellViewer(overlay, width)
	case app.OverlayParameters:
		return model.renderParameters(overlay, width)
	case app.OverlayCellEdit:
		return model.renderCellEditor(overlay, width)
	case app.OverlayRowDetail:
		return model.renderRowDetail(overlay, width)
	case app.OverlayChanges:
		return model.renderChanges(overlay, width)
	case app.OverlayValueFilter:
		return model.renderValueFilter(overlay, width)
	case app.OverlayThemePicker:
		return model.renderThemePicker(overlay, width)
	case app.OverlayActivity:
		return model.renderActivity(connection, overlay, width)
	case app.OverlayExport:
		return model.renderExport(overlay, width)
	case app.OverlayImport:
		return model.renderImport(overlay, width)
	case app.OverlayAiChat:
		return model.renderAiChat(connection, overlay, width)
	case app.OverlayAiChats:
		return model.renderAiChats(connection, overlay, width)
	case app.OverlayPrompt:
		return model.renderPrompt(overlay, width)
	case app.OverlayChoice:
		return model.renderChoice(overlay, width)
	}
	return ""
}

// ListCard says what one card with a list in it draws.
type ListCard struct {
	Kind  app.OverlayKind
	Title string
	// The line at the top that searches the list, or nothing for a list without one.
	Filter string
	// The lines over the rows, which do not scroll and the cursor does not reach.
	Header []string
	Rows   []string
	Cursor int
	Offset int
	Width  int
	// The keys the card names at the bottom, wrapped rather than cut, so a hint whose
	// last key is missing does not read as though the key is not there.
	Keys *KeyLine
	// True while the wheel moved the rows away from the cursor.
	Rolled bool
	// True for a list that says so where the term kept nothing.
	ReportsNoMatch bool
	// The line a card with no rows draws instead of its list, already styled.
	EmptyReport string
	// A note held against the right corner of the top border, already styled.
	Note string
	// How a drag of the bar moves the card. A card that keeps how far it has scrolled in
	// its cursor rather than its offset gives its own, so a drag moves what the keys move.
	Scrolls func(*app.Overlay, int)
	// How many rows the card shows where its height follows what it holds.
	ContentRows int
}

// renderListCard draws a card whose body is a list, with the row under the cursor filled.
// The card keeps a blank row inside each border, and a blank row over the keys it names.
func (model *Model) renderListCard(card ListCard) string {
	content := max(card.Width-4, 1)

	said := card.Keys.buildText()
	hint := []string{}
	if said != "" {
		hint = present.WrapWords(said, content)
	}
	height := model.resolveOverlayHeight(
		card.Kind, card.ContentRows, countHintRows(said, card.Width))

	body := countCardBodyRows(height, len(hint))
	// The line the list is filtered with stands over the rows, so it takes one of them.
	if card.Filter != "" {
		body = max(body-1, 1)
	}
	// So do the lines of the card's own header.
	body = max(body-len(card.Header), 1)

	lines := []string{""}
	if card.Filter != "" {
		lines = append(lines, card.Filter)
	}
	lines = append(lines, card.Header...)

	// Where the rows land in the card, so a press chooses the row it looks like. The top
	// border takes the first row, and the lines written above the list follow it.
	model.layout.overlayRows = rowsHit{
		top:    cardListRow + len(lines),
		offset: scrollFrom(card.Cursor, card.Offset, body, len(card.Rows), card.Rolled),
		from:   1, to: card.Width - 2,
	}

	written := 0
	if len(card.Rows) == 0 {
		if card.EmptyReport != "" {
			lines = append(lines, " "+model.styles.Muted().Render(card.EmptyReport))
			written++
		} else if card.ReportsNoMatch {
			lines = append(lines, " "+model.styles.Muted().Render("no match"))
			written++
		}
	}
	shown := scrollFrom(card.Cursor, card.Offset, body, len(card.Rows), card.Rolled)
	// The bar stands over the last cell of the content, and a list that fits gets none.
	thumb := buildScrollThumb(shown, body, len(card.Rows))
	// The list of every card of this kind is the one the open overlay holds, which is what
	// the keys and the wheel move as well. The border of the block takes its first row and
	// its first column, and the rows of the list follow the lines written above them.
	model.recordCardScrollbar(thumb, 1+card.Width-4, 1+len(lines), min(body, len(thumb)),
		shown, len(card.Rows), card.Scrolls)
	for at := shown; at < len(card.Rows) && written < body; at++ {
		row := card.Rows[at]
		if written < len(thumb) {
			row = model.styles.paintThumbColumn(
				row, thumb[written], card.Width-4, model.styles.Theme.Panel)
		}
		lines = append(lines, row)
		written++
	}
	for written < body {
		row := ""
		if written < len(thumb) {
			row = model.styles.paintThumbColumn(
				row, thumb[written], card.Width-4, model.styles.Theme.Panel)
		}
		lines = append(lines, row)
		written++
	}
	model.layout.overlayRows.count = written
	if len(hint) > 0 {
		lines = append(lines, "")
		// The keys land under the list, one blank column inside the border, and the top
		// border takes the first row of the card.
		for _, line := range model.renderKeyLine(card.Keys, hint,
			cardListRow+len(lines), cardBodyColumn, model.styles.Theme.Panel) {
			lines = append(lines, paintOn(model.styles.Theme.Panel, " ")+line)
		}
	}
	model.rememberCardKeys(card.Keys)
	lines = append(lines, "")

	return model.styles.RenderBox(BoxOptions{
		Width: card.Width, Height: height, Title: card.Title, Note: card.Note,
		Focused: true, Lines: lines,
	})
}

// The two ways a card is drawn: one that removes something carries the mark of a warning.
const (
	destructiveCard = true
	plainCard       = false
)

// renderTextCard draws a card that is as tall as what it holds, with the keys it names at
// the bottom. The card keeps a blank row inside each border, and a blank row over the keys.
func (model *Model) renderTextCard(
	kind app.OverlayKind, title string, width int, lines []string,
	keys *KeyLine, contentRows int, destructive bool,
) string {
	content := max(width-present.CardChrome, 1)
	said := keys.buildText()
	hint := []string{}
	if said != "" {
		hint = present.WrapWords(said, content)
	}
	height := model.resolveOverlayHeight(kind, contentRows, countHintRows(said, width))
	body := countCardBodyRows(height, len(hint))

	written := make([]string, 0, height)
	written = append(written, "")
	for at := range body {
		if at < len(lines) {
			written = append(written, " "+lines[at])
			continue
		}
		written = append(written, "")
	}
	if len(hint) > 0 {
		written = append(written, "")
		// The keys land under the body, one blank column inside the border, and the top
		// border takes the first row of the card.
		for _, line := range model.renderKeyLine(keys, hint,
			cardListRow+len(written), cardBodyColumn, model.styles.Theme.Panel) {
			written = append(written, paintOn(model.styles.Theme.Panel, " ")+line)
		}
	}
	written = append(written, "")
	model.rememberCardKeys(keys)

	return model.styles.RenderBox(BoxOptions{
		Width: width, Height: height, Title: title,
		Focused: true, Destructive: destructive, Lines: written,
	})
}

// Where the body of a card that renderTextCard draws stands in the block of that card: its
// border takes the first row and the first column, it opens with a blank row, and it writes
// each line of its body after one blank column. A card with a list writes its own lines from
// the row under the border, so the list follows them.
const (
	cardBodyRow    = 2
	cardBodyColumn = 2
	cardListRow    = 1
)

// countCardBodyRows returns the rows a card of this height gives its content: its borders,
// the blank row inside each one, and the keys with the blank row over them all come off.
func countCardBodyRows(height, hintLines int) int {
	body := height - present.CardChrome - hintLines
	if hintLines > 0 {
		body--
	}
	if body < 1 {
		return 1
	}
	return body
}

// recordCardBody keeps where the first line of the body of a card was drawn, counted from
// the card itself. The placement carries the whole card onto the screen afterwards, so no
// renderer of a card works out where it landed.
func (model *Model) recordCardBody() {
	model.layout.cardBodyTop = cardBodyRow
	model.layout.cardBodyLeft = cardBodyColumn
}

// The parts of a row every list of a card draws: the padding before the first one, the
// column the scroll bar takes, and the width of the detail where the row has a trail.
const (
	rowPaddingLeft    = 1
	rowScrollbarWidth = 1
	detailBesideTrail = 8
)

// ListRowSpec says what one row of a list holds and how wide each column is.
type ListRowSpec struct {
	// Lead stands before the label in muted text, such as the time a statement ran.
	Lead       string
	LeadWidth  int
	Label      string
	LabelWidth int
	// The glyph of what the row acts on, drawn before its name. A list where no row names
	// one keeps the column for none of them.
	Icon    cfg.IconKind
	HasIcon bool
	Detail  string
	// Trail stands after the detail, at the right of the row. HasTrail is what tells a
	// row with an empty trail from a row without one, which measure the detail
	// differently.
	Trail       string
	HasTrail    bool
	Selected    bool
	Destructive bool
	// Width is the width of the card the row is drawn in.
	Width int
}

// listRowIconWidth is the glyph a row draws before its name, and the blank after it.
const listRowIconWidth = 2

// renderListRow draws one row of a list. The lead, the name and what it does each keep
// their own column, so the names of a list read down the card rather than drifting with
// the length of a detail.
func (model *Model) renderListRow(row ListRowSpec) string {
	theme := model.styles.Theme
	ground := theme.Panel
	quiet, labelInk := theme.Muted, theme.Text
	if row.Destructive {
		labelInk = theme.Error
	}
	if row.Selected {
		ground = theme.Accent
		quiet, labelInk = theme.OnAccent, theme.OnAccent
	}
	paint := func(ink color.Color, text string) string {
		return paintText(ink, ground, text)
	}

	written := paintOn(ground, strings.Repeat(" ", rowPaddingLeft))
	if row.LeadWidth > 0 {
		written += paint(quiet, present.FitText(row.Lead, row.LeadWidth))
	}
	if row.HasIcon {
		iconInk := model.styles.IconColor(row.Icon)
		if row.Selected {
			iconInk = theme.OnAccent
		}
		written += paint(iconInk, present.FitText(
			model.icons.Icon(row.Icon), listRowIconWidth))
	}
	written += paint(labelInk, present.FitText(
		present.TruncateText(row.Label, row.LabelWidth-1), row.LabelWidth))

	if row.HasTrail {
		written += paint(quiet, present.FitText(
			present.TruncateText(row.Detail, detailBesideTrail), detailBesideTrail))
		written += paint(quiet, row.Trail)
	} else {
		room := max(row.Width-present.CardChrome-rowPaddingLeft-
			row.LeadWidth-row.LabelWidth-rowScrollbarWidth, 0)
		written += paint(quiet, present.TruncateText(row.Detail, room))
	}

	// The card keeps a blank column of its own ground on each side, and the scroll bar
	// stands in the column before the last one, so the row ends there.
	pad := paintOn(theme.Panel, " ")
	return pad + padStyledOn(written, row.Width-4, ground) + pad + pad
}

// The columns of one row of the help. A narrower key column would wrap a chord onto a
// second line. A row the search kept names its group, so its text column is narrower.
const (
	helpKeyWidth     = 26
	helpCardWidth    = 78
	helpTextWidth    = helpCardWidth - helpKeyWidth - 6
	helpSectionWidth = 16
	helpFoundWidth   = helpTextWidth - helpSectionWidth
)

// The name column of the lists that carry no key of their own.
const (
	themeLabelWidth    = 26
	activityLabelWidth = 20
	valueMarkWidth     = 2
	valueLabelWidth    = 34
)

// selectedThemeMark stands on the theme that was applied when the picker opened.
const selectedThemeMark = "✓"

// The columns of one row of a menu: the key it is bound to, and its name.
const (
	menuChordWidth = 13
	menuLabelWidth = 22
	// The menus that carry no key of their own give the name the room instead.
	copyLabelWidth   = 20
	objectLabelWidth = 24
)

// The columns of one row of the palette, the history and the saved statements.
const (
	paletteChordWidth = 13
	paletteLabelWidth = 38
	historyTimeWidth  = 12
	historySQLWidth   = 74
	savedNameWidth    = 26
)

// readOverlayTerm returns the term typed into the line at the top of a list.
func (model *Model) readOverlayTerm(overlay app.Overlay) string {
	if overlay.Draft == nil {
		return ""
	}
	return strings.TrimSpace(overlay.Draft.Text)
}

// renderSearchField draws the same line with the mark of a search rather than of a filter,
// which is what the help draws.
func (model *Model) renderSearchField(
	overlay app.Overlay, width int, placeholder string,
) string {
	return model.renderFilterLine(overlay, width, placeholder, -1, " / ")
}

// renderFilterFieldOf draws the same field with the word it asks for, and the count of
// the rows it kept where the list shows one.
func (model *Model) renderFilterFieldOf(
	overlay app.Overlay, width int, placeholder string, count int,
) string {
	return model.renderFilterLine(overlay, width, placeholder, count,
		" "+model.icons.Icon(cfg.IconPrompt)+" ")
}

func (model *Model) renderFilterLine(
	overlay app.Overlay, width int, placeholder string, count int, mark string,
) string {
	theme := model.styles.Theme
	if overlay.Draft == nil {
		return ""
	}
	written := overlay.Draft.Text
	body := paintText(theme.Text, theme.Header, written)
	if written == "" {
		body = model.styles.Muted().Background(theme.Header).Render(placeholder)
	}
	caret := paintOn(theme.Accent, " ")
	counted := ""
	if count >= 0 {
		counted = model.styles.Muted().Background(theme.Header).
			Render(" " + present.FormatCount(int64(count)) + " ")
	}

	field := paintText(theme.Accent, theme.Header, mark) + body + caret
	// The line runs the width of the content, so the ground of the card stands beside it
	// where the padding of the card is.
	gap := max(width-4-measureStyledWidth(field)-measureStyledWidth(counted), 0)
	pad := paintOn(theme.Panel, " ")
	return pad + field + paintOn(theme.Header, strings.Repeat(" ", gap)) + counted + pad
}

// helpRow is one row of the help: a key and what it does, under the name of a section.
type helpRow struct {
	Section string
	Chord   string
	Label   string
}

// buildHelpRows returns every row of the help, in the order of the groups.
func (model *Model) buildHelpRows() []helpRow {
	rows := []helpRow{}
	for _, section := range model.listHelpSections() {
		for _, entry := range section.Entries {
			chord := model.describeHelpKeys(entry)
			if chord == "" {
				continue
			}
			rows = append(rows, helpRow{
				Section: section.Title, Chord: chord, Label: entry.Text,
			})
		}
	}
	return rows
}

// findHelpRows returns the rows whose keys, text or group name hold the term.
func (model *Model) findHelpRows(term string) []helpRow {
	kept := []helpRow{}
	for _, row := range model.buildHelpRows() {
		if present.MatchesText(row.Chord+" "+row.Label+" "+row.Section, term) {
			kept = append(kept, row)
		}
	}
	return kept
}

// describeHelpKeys writes the keys of one help row. A row of actions names the chord
// each is bound to now, so a rebound key moves the help with it.
func (model *Model) describeHelpKeys(entry HelpEntry) string {
	if len(entry.Actions) == 0 {
		return entry.Keys
	}
	written := []string{}
	for _, action := range entry.Actions {
		if chord := model.registry.FormatActionChord(entry.Scope, action); chord != "" {
			written = append(written, chord)
		}
	}
	return strings.Join(written, "  ")
}

// helpPlaceholder is what the search line of the help asks for.
const helpPlaceholder = "a key, or what it does"

// scrollHelpByCursor moves the help, which scrolls without a cursor of its own and so keeps
// how far it has scrolled where a list keeps its cursor. Without this a drag of its bar writes
// an offset the card never reads, and the bar returns a drag by standing still.
func scrollHelpByCursor(overlay *app.Overlay, offset int) {
	overlay.List.Cursor, overlay.List.Offset = offset, offset
}

// renderHelp draws every key that is bound, grouped the way a reader looks for one. A term
// drops the groups, and names the group of every row it kept.
func (model *Model) renderHelp(overlay app.Overlay, width int) string {
	term := model.readOverlayTerm(overlay)
	filter := model.renderSearchField(overlay, width, helpPlaceholder)

	if term != "" {
		found := model.findHelpRows(term)
		written := make([]string, 0, len(found))
		for _, row := range found {
			written = append(written, model.renderFoundHelpRow(row, width))
		}
		return model.renderListCard(ListCard{
			Kind: app.OverlayHelp, Title: " help ", Filter: filter, Rows: written,
			Cursor: overlay.List.Cursor, Offset: overlay.List.Cursor, Width: width,
			Scrolls: scrollHelpByCursor,
			Keys: model.sayKeys().
				say(present.FormatCount(int64(len(found)))+" of the keys").
				bind(cfg.ScopeDialog, ActionClose, "close"),
		})
	}

	written := []string{}
	for _, section := range model.listHelpSections() {
		written = append(written, paintText(model.styles.Theme.AccentAlt, model.styles.Theme.Panel, " "+section.Title))
		for _, entry := range section.Entries {
			// An action nothing is bound to has no row of its own: it is reached from the
			// palette until a chord is given to it.
			chord := model.describeHelpKeys(entry)
			if chord == "" {
				continue
			}
			written = append(written, model.renderHelpRow(chord, entry.Text, width))
		}
		// Every group keeps a blank row under it, the last one too.
		written = append(written, "")
	}
	return model.renderListCard(ListCard{
		Kind: app.OverlayHelp, Title: " help ", Filter: filter, Rows: written,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Cursor, Width: width,
		Scrolls: scrollHelpByCursor,
		Keys: model.sayKeys().say("type to search").
			bind(cfg.ScopeDialog, ActionClose, "close"),
	})
}

// renderHelpRow draws one row of the help: the keys, then what they do.
func (model *Model) renderHelpRow(chord, text string, width int) string {
	theme := model.styles.Theme
	room := max(width-4-helpKeyWidth, 0)
	written := paintText(theme.Accent, theme.Panel, present.FitText("  "+chord, helpKeyWidth)) +
		paintText(theme.Text, theme.Panel, present.TruncateText(text, room))
	return padStyledOn(" "+written, width-1, theme.Panel) +
		paintOn(theme.Panel, " ")
}

// renderFoundHelpRow draws one row the search kept, which names its group on the right.
func (model *Model) renderFoundHelpRow(row helpRow, width int) string {
	theme := model.styles.Theme
	written := paintText(theme.Accent, theme.Panel, present.FitText("  "+row.Chord, helpKeyWidth)) +
		paintText(theme.Text, theme.Panel, present.FitText(
			present.TruncateText(row.Label, helpFoundWidth-1), helpFoundWidth)) +
		paintText(theme.Faint, theme.Panel, present.TruncateText(row.Section, helpSectionWidth))
	return padStyledOn(" "+written, width-1, theme.Panel) +
		paintOn(theme.Panel, " ")
}

// renderPalette draws every action the palette can run, with the key of each.
func (model *Model) renderPalette(overlay app.Overlay, width int) string {
	actions := model.filterPalette(overlay)
	rows := make([]string, 0, len(actions))
	for at, action := range actions {
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: action.Chord, LeadWidth: paletteChordWidth,
			Label: action.Label, LabelWidth: paletteLabelWidth,
			Detail: action.Detail, Selected: at == overlay.List.Cursor, Width: width,
		}))
	}
	return model.renderListCard(ListCard{
		Kind: app.OverlayPalette, Title: " command palette ",
		Filter: model.renderFilterFieldOf(overlay, width, "action", -1), Rows: rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ReportsNoMatch: true,
		Keys: model.sayKeys().say("type to filter").
			bind(cfg.ScopeList, ActionChooseRow, "run").
			bind(cfg.ScopeDialog, ActionClose, "close"),
	})
}

// renderHistory draws every statement that ran on this profile, newest first.
func (model *Model) renderHistory(overlay app.Overlay, width int) string {
	entries := model.filterHistory(overlay)
	rows := make([]string, 0, len(entries))
	now := time.Now()

	for at, entry := range entries {
		// The count of the rows keeps a column and the time it took stands at the right,
		// so the statements above one another read as a list rather than a paragraph.
		outcome := ""
		switch {
		case entry.ErrorMessage != "":
			outcome = "failed"
		case entry.HasRowCount:
			outcome = present.FormatCount(entry.RowCount)
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: present.FormatWhen(entry.RanAt, now), LeadWidth: historyTimeWidth,
			Label: core.CollapseWhitespace(entry.SQL), LabelWidth: historySQLWidth,
			Detail: outcome, Trail: present.FormatDuration(entry.Elapsed), HasTrail: true,
			Selected: at == overlay.List.Cursor, Destructive: entry.ErrorMessage != "",
			Width: width,
		}))
	}

	keys := model.sayKeys().
		bind(cfg.ScopeList, ActionChooseRow, "into this tab").
		bind(cfg.ScopeDialog, ActionOpenInNewTab, "into a new tab").
		bind(cfg.ScopeDialog, ActionClose, "close")
	return model.renderListCard(ListCard{
		Kind: app.OverlayHistory, Title: " query history ",
		Filter: model.renderFilterFieldOf(
			overlay, width, "search the statements", len(entries)),
		Rows: rows, Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ReportsNoMatch: true, Keys: keys,
	})
}

// savedSQLWidth is how much of a saved statement the row shows, and the filter reads.
const savedSQLWidth = 70

// The lead column of the saved statements, which marks the ones a project file holds.
const (
	savedProjectLead = "project"
	savedLeadWidth   = 8
)

// renderSaved draws the statements a reader kept by name, and the ones the project file
// holds for the whole team.
func (model *Model) renderSaved(overlay app.Overlay, width int) string {
	queries := model.filterSaved(overlay)
	// The lead column stands empty where no statement comes from a project file, so a
	// user without one loses no room to it.
	leadWidth := 0
	if slices.ContainsFunc(overlay.Saved, func(saved app.SavedRow) bool {
		return saved.IsFromProject()
	}) {
		leadWidth = savedLeadWidth
	}

	rows := make([]string, 0, len(queries))
	for at, saved := range queries {
		lead := ""
		if saved.IsFromProject() {
			lead = savedProjectLead
		}
		detail := core.CollapseWhitespace(saved.SQL)
		if saved.Description != "" {
			detail = saved.Description
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: lead, LeadWidth: leadWidth,
			Label: saved.Name, LabelWidth: savedNameWidth,
			Detail:   present.TruncateText(detail, savedSQLWidth),
			Selected: at == overlay.List.Cursor, Width: width,
		}))
	}
	keys := model.sayKeys().
		bind(cfg.ScopeList, ActionChooseRow, "load").
		bind(cfg.ScopeDialog, ActionListSecondary, "delete").
		bind(cfg.ScopeDialog, ActionClose, "close")
	return model.renderListCard(ListCard{
		Kind:   app.OverlaySaved,
		Title:  " saved queries · " + present.FormatCount(int64(len(overlay.Saved))) + " ",
		Filter: model.renderFilterFieldOf(overlay, width, "name", -1), Rows: rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ReportsNoMatch: true, Keys: keys,
	})
}

// renderMenu draws a menu of actions, one row each.
func (model *Model) renderMenu(overlay app.Overlay, width int) string {
	actions := model.filterMenu(overlay)
	// A menu whose rows carry no key of their own drops the column that would hold one.
	chordWidth := 0
	for _, action := range overlay.Actions {
		if action.Chord != "" {
			chordWidth = menuChordWidth
			break
		}
	}

	labelWidth := menuLabelWidth
	switch overlay.Kind {
	case app.OverlayCopyMenu:
		labelWidth = copyLabelWidth
	case app.OverlayObjectMenu:
		labelWidth = objectLabelWidth
	}

	// A menu whose rows all act on the same thing drops the column that would hold a glyph,
	// and so does one whose set draws no glyph at all.
	icons := false
	for _, action := range overlay.Actions {
		if model.icons.Icon(action.Icon) != "" {
			icons = true
			break
		}
	}
	if icons {
		labelWidth -= listRowIconWidth
	}

	rows := make([]string, 0, len(actions))
	for at, action := range actions {
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: action.Chord, LeadWidth: chordWidth,
			Icon: action.Icon, HasIcon: icons,
			Label: action.Label, LabelWidth: labelWidth, Detail: action.Detail,
			Selected: at == overlay.List.Cursor, Destructive: action.Destructive, Width: width,
		}))
	}
	title := overlay.Title
	if title == "" {
		title = " menu "
	} else if !strings.HasPrefix(title, " ") {
		title = " " + title + " "
	}
	taken, placeholder := "runs it", "action"
	switch overlay.Kind {
	case app.OverlayCopyMenu:
		taken, placeholder = "copy", "what to copy"
	case app.OverlayObjectMenu:
		taken = "writes the query into the editor"
	}
	said := model.sayKeys().
		bind(cfg.ScopeList, ActionChooseRow, taken).
		bind(cfg.ScopeDialog, ActionClose, "close")
	return model.renderListCard(ListCard{
		Kind: overlay.Kind, Title: title,
		Filter: model.renderFilterFieldOf(overlay, width, placeholder, -1), Rows: rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ReportsNoMatch: true, Keys: said,
	})
}

// The columns of one answer of a question with more than two of them.
const (
	choiceKeyWidth   = 4
	choiceLabelWidth = 22
)

// renderConfirm draws a question with two answers. The answers are drawn as well as said,
// so the question can be answered with a pointer and not only with a letter.
func (model *Model) renderConfirm(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	lines := model.wrapErrorText(overlay.Body, width-present.CardChrome)

	yes := paintText(model.styles.InkOn(theme.Error), theme.Error, "  "+
		model.registry.FormatActionChords(cfg.ScopeDialog, ActionAnswerYes)+" run  ")
	no := paintText(theme.Text, theme.Header, "  "+
		model.registry.FormatActionChords(cfg.ScopeDialog, ActionAnswerNo)+" cancel  ")
	lines = append(lines, "", yes+"  "+no)

	keys := model.sayKeys().
		bind(cfg.ScopeDialog, ActionAnswerYes, "run").
		bind(cfg.ScopeDialog, ActionAnswerNo, "cancel").
		bind(cfg.ScopeDialog, ActionClose, "cancel")
	model.recordCardBody()
	model.recordAnswerChips(len(lines)-1, measureStyledWidth(yes), measureStyledWidth(no))
	return model.renderTextCard(overlay.Kind, overlay.Title, width, lines, keys, 0, destructiveCard)
}

// recordAnswerChips keeps where the two answers of a question were drawn, so a press on one
// of them returns it. The yes chip stands first, two blanks apart from the no chip.
func (model *Model) recordAnswerChips(line, yesWidth, noWidth int) {
	row := model.layout.cardBodyTop + line
	left := model.layout.cardBodyLeft
	model.layout.overlayChips = []chipHit{
		{index: 0, row: row, from: left, to: left + yesWidth - 1},
		{index: 1, row: row, from: left + yesWidth + 2,
			to: left + yesWidth + 2 + noWidth - 1},
	}
}

// renderChoice draws a question with more than two answers, each on its own letter.
func (model *Model) renderChoice(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	lines := model.wrapText(overlay.Body, width-present.CardChrome)
	lines = append(lines, "")
	firstChoice := len(lines)
	for _, choice := range overlay.Choices {
		ink := theme.Text
		if choice.Destructive {
			ink = theme.Error
		}
		lines = append(lines,
			paintText(theme.Accent, theme.Panel, present.FitText(" "+choice.Key, choiceKeyWidth))+
				paintText(ink, theme.Panel, present.FitText(choice.Label, choiceLabelWidth))+
				model.styles.Muted().Render(choice.Detail))
	}
	keys := model.sayKeys().bind(cfg.ScopeDialog, ActionClose, "stay here")
	model.recordCardBody()
	model.layout.formRows = rowsHit{
		top: model.layout.cardBodyTop + firstChoice, count: len(overlay.Choices),
		from: model.layout.cardBodyLeft - 1,
		to:   model.layout.cardBodyLeft + width - present.CardChrome,
	}
	return model.renderTextCard(overlay.Kind, overlay.Title, width, lines, keys, 0, plainCard)
}

// renderMessage draws a block of text that has to be read.
func (model *Model) renderMessage(overlay app.Overlay, width int) string {
	lines := []string{}
	source := overlay.Lines
	if len(source) == 0 {
		source = strings.Split(overlay.Body, "\n")
	}
	for _, line := range source {
		lines = append(lines, model.wrapText(line, width-present.CardChrome)...)
	}
	return model.renderTextCard(overlay.Kind, overlay.Title, width, lines,
		model.sayKeys().bind(cfg.ScopeDialog, ActionClose, "close"), len(lines), plainCard)
}

// renderDiagram draws the lines of an ER diagram. A line keeps its own shape and is never
// wrapped, because a box drawn over two rows would come apart.
func (model *Model) renderDiagram(overlay app.Overlay, width int) string {
	room := max(width-present.CardChrome, 1)
	lines := make([]string, 0, len(overlay.Lines))
	for at := overlay.List.Cursor; at < len(overlay.Lines); at++ {
		line := overlay.Lines[at]
		if overlay.List.Offset < len([]rune(line)) {
			line = string([]rune(line)[overlay.List.Offset:])
		} else {
			line = ""
		}
		lines = append(lines, model.styles.Ink().Render(present.TruncateText(line, room)))
	}
	return model.renderTextCard(overlay.Kind, overlay.Title, width, lines,
		model.sayKeys().bind(cfg.ScopeDialog, ActionClose, "close").name("↑↓ ←→", "scroll"),
		len(overlay.Lines), plainCard)
}

// renderCellViewer draws one cell full size. A value taller than the card scrolls inside it,
// and the bar down its last column says how much of it is on screen.
func (model *Model) renderCellViewer(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	room := max(width-present.CardChrome, 1)
	written := present.FormatForViewer(overlay.Cell.Value, overlay.Cell.Column.DataType)
	held := []string{}
	for line := range strings.SplitSeq(present.SafeLines(written), "\n") {
		held = append(held, model.wrapText(line, room)...)
	}

	// The type and the size of the value stand in the footer, so the title names the
	// column and nothing else.
	counted := present.FormatCountOf(int64(len(strings.Split(written, "\n"))), "line", "lines")
	named := overlay.Cell.Column.DataType
	// A JSON value is drawn indented, so the footer says so.
	if present.IsJSONType(overlay.Cell.Column.DataType) {
		named += " · prettified"
	}
	keys := model.sayKeys().say(overlay.Notice).say(named).say(counted).
		bind(cfg.ScopeDialog, ActionCopyValue, "copy").
		bind(cfg.ScopeDialog, ActionClose, "close")

	said := keys.buildText()
	height := model.resolveOverlayHeight(overlay.Kind, len(held), countHintRows(said, width))
	body := countCardBodyRows(height, len(present.WrapWords(said, room)))
	ink := theme.Text
	if written == core.NullText {
		ink = theme.Muted
	}
	lines := model.scrollCardRows(len(held), overlay.List.Offset, body, room, theme.Panel,
		func(at int) string {
			return paintText(ink, theme.Panel, present.TruncateText(held[at], room))
		})
	return model.renderTextCard(overlay.Kind, " "+overlay.Cell.Column.Name+" ", width, lines,
		keys, len(held), plainCard)
}

// renderParameters draws the field the values of the `:name` marks are written in, as JSON,
// so a null stays a null and a number stays a number.
func (model *Model) renderParameters(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	keys := model.buildParameterKeys(overlay)
	said := keys.buildText()
	height := model.resolveOverlayHeight(
		overlay.Kind, overlay.ContentRows, countHintRows(said, width))
	lines := model.renderDraftRows(overlay.Draft, width-present.CardChrome,
		countCardBodyRows(height, len(present.WrapWords(said, width-present.CardChrome))),
		FieldLook{Ground: theme.Panel, Ink: theme.Text, Focused: true})

	title := " " + present.FormatCountOf(
		int64(len(overlay.Names)), "parameter", "parameters") + " "
	return model.renderTextCard(overlay.Kind, title, width, lines,
		keys, overlay.ContentRows, plainCard)
}

// buildParameterKeys names every key of the values card, with the chord each one is bound to.
func (model *Model) buildParameterKeys(overlay app.Overlay) *KeyLine {
	return model.sayKeys().say(overlay.Notice).
		bind(cfg.ScopeDialog, ActionRunWithValues, "run").
		bind(cfg.ScopeDialog, ActionPrettifyJSON, "prettify JSON").
		bind(cfg.ScopeDialog, ActionClose, "cancel")
}

// renderCellEditor draws the list a cell is picked from, or the field one cell or a whole new
// row is written in.
func (model *Model) renderCellEditor(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	keys := model.buildCellEditorKeys(overlay)
	said := keys.buildText()

	lines := model.renderCellChoices(overlay)
	if len(overlay.Cell.Choices) == 0 {
		height := model.resolveOverlayHeight(
			overlay.Kind, overlay.ContentRows, countHintRows(said, width))
		lines = model.renderDraftRows(overlay.Draft, width-present.CardChrome,
			countCardBodyRows(height, len(present.WrapWords(said, width-present.CardChrome))),
			FieldLook{Ground: theme.Panel, Ink: theme.Text, Focused: true})
	}
	return model.renderTextCard(overlay.Kind, model.buildCellEditorTitle(overlay), width,
		lines, keys, overlay.ContentRows, plainCard)
}

// buildCellEditorTitle names the column and the type the value is written for. A type that
// holds JSON is named as such, because the card prettifies one.
func (model *Model) buildCellEditorTitle(overlay app.Overlay) string {
	title := overlay.Cell.Column.Name + " · " + overlay.Cell.Column.DataType
	if present.IsJSONType(overlay.Cell.Column.DataType) {
		title += " · json"
	}
	return " " + title + " "
}

// buildCellEditorKeys names every key of the card, with the chord each one is bound to. A
// cell that is picked has no field, so it offers neither a save chord nor the JSON key.
func (model *Model) buildCellEditorKeys(overlay app.Overlay) *KeyLine {
	picking := len(overlay.Cell.Choices) > 0
	keys := model.sayKeys().say(overlay.Notice)
	switch {
	case picking:
		keys.name("↑↓", "pick").bind(cfg.ScopeList, ActionChooseRow, "save")
	default:
		keys.bind(cfg.ScopeDialog, ActionSaveCell, "save")
		if present.IsJSONType(overlay.Cell.Column.DataType) {
			keys.bind(cfg.ScopeDialog, ActionPrettifyJSON, "prettify JSON")
		}
	}
	return keys.
		bind(cfg.ScopeDialog, ActionSetNull, "NULL").
		bind(cfg.ScopeDialog, ActionSetEmpty, "empty").
		bind(cfg.ScopeDialog, ActionSetDefault, "default").
		bind(cfg.ScopeDialog, ActionClose, "cancel")
}

// renderCellChoices draws the values the column takes, one to a row, with the one the cursor
// stands on marked.
func (model *Model) renderCellChoices(overlay app.Overlay) []string {
	theme := model.styles.Theme
	lines := make([]string, 0, len(overlay.Cell.Choices))
	for at, value := range overlay.Cell.Choices {
		ink, ground := theme.Text, theme.Background
		if at == overlay.List.Cursor {
			ink, ground = theme.OnAccent, theme.BorderFocus
		}
		lines = append(lines, paintText(ink, ground, " "+value))
	}
	return lines
}

// renderRowDetail draws every column of one row, so a wide row is read without scrolling.
func (model *Model) renderRowDetail(overlay app.Overlay, width int) string {
	if overlay.Window.Index < 0 || overlay.Window.Index >= len(overlay.Window.Rows) {
		return ""
	}
	theme := model.styles.Theme
	row := overlay.Window.Rows[overlay.Window.Index]
	inner := width - present.CardChrome
	plan := present.PlanFieldColumns(inner)
	room := max(inner-plan.Name-plan.Type, 1)

	// A value too long for its column wraps under itself, so the whole of it is read
	// without the two names giving way.
	held := []string{}
	for at, column := range overlay.Window.Columns {
		var value any
		if at < len(row) {
			value = row[at]
		}
		written := present.SafeLines(present.FormatForViewer(value, column.DataType))
		ink := theme.Text
		if value == nil {
			ink = theme.Muted
		}
		ground := theme.Panel
		if at%2 == 1 {
			ground = theme.Zebra
		}
		head := paintText(theme.Accent, ground, present.FitText(column.Name, plan.Name)) +
			paintText(theme.Muted, ground, present.FitText(column.DataType, plan.Type))
		for line, text := range model.wrapText(written, room) {
			if line > 0 {
				head = paintOn(ground, strings.Repeat(" ", plan.Name+plan.Type))
			}
			held = append(held, padStyledOn(
				head+paintText(ink, ground, present.TruncateText(text, room)), inner, ground))
		}
	}

	keys := model.sayKeys().name("←→", "another row").name("↑↓", "scroll").
		bind(cfg.ScopeDialog, ActionClose, "close")
	said := keys.buildText()
	height := model.resolveOverlayHeight(
		overlay.Kind, len(overlay.Window.Columns), countHintRows(said, width))
	body := countCardBodyRows(height, len(present.WrapWords(said, inner)))
	lines := model.scrollCardRows(len(held), overlay.List.Offset, body, inner, theme.Panel,
		func(at int) string { return held[at] })

	title := " row " + strconv.Itoa(overlay.Window.Index+1) + " of " +
		strconv.Itoa(len(overlay.Window.Rows)) + " "
	return model.renderTextCard(overlay.Kind, title, width, lines,
		keys, len(overlay.Window.Columns), plainCard)
}

// scrollCardRows returns the rows a card that scrolls without a cursor draws: the offset
// clamped to what is there, the bar recorded and its thumb written over the last column of
// each row. `buildRow` draws one row of the content, so a card that is scrolled past draws
// only the rows it shows.
func (model *Model) scrollCardRows(
	count, offset, body, room int, ground color.Color, buildRow func(int) string,
) []string {
	from := clampOffset(offset, body, count)
	// The keys that scroll the card read these, because only the draw knows how many rows
	// the content takes at this width.
	model.layout.cardLines, model.layout.cardBody = count, body

	thumb := buildScrollThumb(from, body, count)
	model.recordCardScrollbar(thumb, cardBodyColumn+room-1, cardBodyRow,
		min(body, len(thumb)), from, count, nil)
	lines := make([]string, 0, body)
	for at := from; at < count && len(lines) < body; at++ {
		line := buildRow(at)
		if len(lines) < len(thumb) {
			line = model.styles.paintThumbColumn(line, thumb[len(lines)], room-1, ground)
		}
		lines = append(lines, line)
	}
	return lines
}

// recordCardScrollbar keeps where the bar of a card that scrolls without a cursor was drawn.
// The caller records the body of the card first, because only the sums that drew it know
// where it landed.
func (model *Model) recordCardScrollbar(
	thumb []string, column, top, rows, from, total int, scrolls func(*app.Overlay, int),
) {
	connection := model.Active()
	if len(thumb) == 0 || connection == nil {
		return
	}
	if scrolls == nil {
		scrolls = scrollCardByOffset
	}
	held := &connection.Overlay
	model.recordScrollbar(column, top, rows, from, total,
		func(offset int) tea.Cmd {
			scrolls(held, offset)
			return nil
		})
}

// scrollCardByOffset moves a card that keeps how far it has scrolled in its offset, which is
// every card whose list has a cursor of its own.
func scrollCardByOffset(overlay *app.Overlay, offset int) {
	overlay.List.Offset, overlay.List.Rolled = offset, true
}

// rowsPerChange is how many rows one staged change takes in the review.
const rowsPerChange = 3

// renderChanges draws the staged work as the statements that will run, with their bind values.
func (model *Model) renderChanges(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	inner := width - present.CardChrome
	lines := []string{}

	if len(overlay.Changes) == 0 {
		lines = append(lines, model.styles.Muted().Render("nothing staged"))
	}
	// Each change takes three rows, on a ground that steps with it, so one change is
	// read apart from the next.
	for at, change := range overlay.Changes {
		ground := theme.Panel
		if at%2 == 1 {
			ground = theme.Zebra
		}
		written := make([]string, 0, len(change.Params))
		for _, value := range change.Params {
			written = append(written, core.WriteJSONValue(value))
		}
		// The indent of a row is its own, so only the text of the row is collapsed: a
		// statement written over several lines still takes one row.
		for _, row := range []struct {
			indent, text string
			ink          color.Color
		}{
			{" ", change.Description, theme.Text},
			{"   ", change.Display, theme.Muted},
			{"   ", "binds: " + strings.Join(written, ", "), theme.Muted},
		} {
			drawn := present.TruncateText(
				row.indent+core.CollapseWhitespace(row.text), inner)
			lines = append(lines,
				padStyledOn(paintText(row.ink, ground, drawn), inner, ground))
		}
	}

	keys := model.sayKeys().
		bind(cfg.ScopeDialog, ActionApplyChanges, "apply").
		bind(cfg.ScopeDialog, ActionDiscardChanges, "discard").
		bind(cfg.ScopeDialog, ActionClose, "close")
	return model.renderTextCard(overlay.Kind,
		" pending changes · "+present.FormatCount(int64(len(overlay.Changes)))+" ",
		width, lines, keys, max(len(overlay.Changes)*rowsPerChange, 1), destructiveCard)
}

// renderValueFilter draws the values of one column, and which of them stay on screen.
func (model *Model) renderValueFilter(overlay app.Overlay, width int) string {
	rows := make([]string, 0, len(overlay.Values))
	for at, value := range overlay.Values {
		mark := "☐"
		if overlay.Kept[value.Value] {
			mark = "☑"
		}
		written := value.Value
		if written == "" {
			written = "(empty)"
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: mark, LeadWidth: valueMarkWidth,
			Label: written, LabelWidth: valueLabelWidth,
			Detail:   present.FormatCount(int64(value.Count)),
			Selected: at == overlay.List.Cursor, Width: width,
		}))
	}

	keys := model.sayKeys().
		bind(cfg.ScopeDialog, ActionToggleValue, "pick").
		bind(cfg.ScopeDialog, ActionKeepOnlyValue, "only this").
		bind(cfg.ScopeDialog, ActionKeepAllValues, "all").
		bind(cfg.ScopeList, ActionChooseRow, "apply").
		bind(cfg.ScopeDialog, ActionClose, "cancel")
	return model.renderListCard(ListCard{
		Kind: app.OverlayValueFilter, Title: overlay.Title, Rows: rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ContentRows: max(len(overlay.Values), 1), Keys: keys,
	})
}

// renderThemePicker draws every theme, and the one applied.
func (model *Model) renderThemePicker(overlay app.Overlay, width int) string {
	choices := model.filterThemes(overlay)
	rows := make([]string, 0, len(choices))
	for at, choice := range choices {
		// The theme the picker opened on carries the mark, so the reader knows what to
		// go back to while stepping through the rest.
		trail := ""
		if choice.Name == overlay.Body {
			trail = selectedThemeMark
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Label: choice.Title, LabelWidth: themeLabelWidth,
			Detail: string(choice.Appearance), Trail: trail, HasTrail: true,
			Selected: at == overlay.List.Cursor, Width: width,
		}))
	}
	return model.renderListCard(ListCard{
		Kind: app.OverlayThemePicker, Title: " theme ",
		Filter: model.renderFilterFieldOf(overlay, width, "theme", len(choices)), Rows: rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ReportsNoMatch: true,
		Keys: model.sayKeys().say("move to try one").
			bind(cfg.ScopeList, ActionChooseRow, "select").
			bind(cfg.ScopeDialog, ActionClose, "cancel"),
	})
}

// The columns of the dashboard: the meter of the connections, and the clock at the right of
// a row of the blocking tree.
const (
	dashboardMeterWidth = 10
	dashboardGap        = "   "
	// dashboardBusyShare is the share of the connection limit that reads as a warning.
	dashboardBusyShare = 0.8
)

// renderActivity draws what the server is doing: the load it is under, the sessions waiting
// for a lock, and every session it holds.
func (model *Model) renderActivity(
	connection *app.Connection, overlay app.Overlay, width int,
) string {
	rows := make([]string, 0, len(overlay.Sessions))
	for at, session := range overlay.Sessions {
		application := session.ApplicationName
		if application == "" {
			application = "?"
		}
		detail := core.FormatClock(session.Duration) + " · " +
			session.User + "@" + application + " · " +
			present.TruncateText(core.CollapseWhitespace(session.Query), 60)
		rows = append(rows, model.renderListRow(ListRowSpec{
			Label:      strconv.FormatInt(session.PID, 10) + " " + session.State,
			LabelWidth: activityLabelWidth, Detail: detail,
			Selected: at == overlay.List.Cursor, Destructive: session.State == "active",
			Width: width,
		}))
	}

	profile := connection.Profile()
	keys := model.sayKeys().
		bind(cfg.ScopeList, ActionChooseRow, "open statement").
		bind(cfg.ScopeDialog, ActionStopSession, "stop statement").
		bind(cfg.ScopeDialog, ActionListSecondary, "end session")
	if len(overlay.Server.Locks) > 0 {
		keys = keys.bind(cfg.ScopeDialog, ActionFoldRow, "fold").
			bind(cfg.ScopeDialog, ActionUnfoldRow, "open")
	}
	keys = keys.bind(cfg.ScopeDialog, ActionClose, "close")

	return model.renderListCard(ListCard{
		Kind:   app.OverlayActivity,
		Title:  " " + buildDashboardTitle(profile) + " ",
		Note:   model.renderServerUptime(overlay.Server),
		Header: model.buildDashboardHeader(overlay, width),
		Rows:   rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset,
		Rolled: overlay.List.Rolled, Width: width,
		EmptyReport: "the server holds no other session",
		Keys:        keys,
	})
}

// buildDashboardTitle names the connection the card is watching, its environment, and how
// often it refreshes.
func buildDashboardTitle(profile cfg.Profile) string {
	said := present.SafeText(profile.Name)
	if profile.Environment != "" {
		said += " · " + string(profile.Environment)
	}
	return said + " · refreshing " + core.FormatLargestUnit(dashboardRefreshWait)
}

// renderServerUptime returns how long the server has been up, for the right of the title.
func (model *Model) renderServerUptime(reading app.ServerReading) string {
	if !reading.HasLoad || reading.Load.StartedAt.IsZero() {
		return ""
	}
	return model.styles.Muted().Render(
		" uptime " + core.FormatLargestUnit(time.Since(reading.Load.StartedAt)) + " ")
}

// buildDashboardHeader returns the lines over the list: what the server is carrying, and
// the sessions waiting for a lock. A panel the server did not answer for is left out.
func (model *Model) buildDashboardHeader(overlay app.Overlay, width int) []string {
	lines := []string{}
	if summary := model.buildDashboardSummary(overlay, width); summary != "" {
		lines = append(lines, summary)
	}
	lines = append(lines, model.buildBlockingPanel(overlay, width)...)
	lines = append(lines, model.buildSlowPanel(overlay, width)...)
	if len(lines) == 0 {
		return lines
	}
	theme := model.styles.Theme
	return append(lines, model.renderPanelLine(
		paintText(theme.Faint, theme.Panel,
			strings.Repeat("─", max(width-present.CardChrome, 0))), width))
}

// buildDashboardSummary returns the one line of what the server is carrying now.
func (model *Model) buildDashboardSummary(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	said := paintOn(theme.Panel, strings.Repeat(" ", rowPaddingLeft))

	reading := overlay.Server
	if reading.HasLoad {
		load := reading.Load
		said += paintText(theme.Muted, theme.Panel, "conns ")
		said += paintText(model.resolveLoadInk(load), theme.Panel,
			strconv.FormatInt(load.Connections, 10)+"/"+
				strconv.FormatInt(load.MaxConnections, 10))
		said += paintOn(theme.Panel, " ")
		said += paintText(model.resolveLoadInk(load), theme.Panel, present.BuildMeter(
			float64(load.Connections), float64(load.MaxConnections), dashboardMeterWidth))
		said += paintOn(theme.Panel, dashboardGap)
	}

	said += paintText(theme.Muted, theme.Panel, "sessions ")
	said += paintText(theme.Text, theme.Panel, strconv.Itoa(len(overlay.Sessions)))

	if reading.HasLocks {
		waiting := app.CountBlockedSessions(reading.Locks)
		ink := theme.Text
		if waiting > 0 {
			ink = theme.Error
		}
		said += paintOn(theme.Panel, dashboardGap)
		said += paintText(theme.Muted, theme.Panel, "locks ")
		said += paintText(ink, theme.Panel, strconv.Itoa(waiting)+" waiting")
	}
	for _, measure := range model.buildDashboardMeasures(overlay) {
		said += paintOn(theme.Panel, dashboardGap)
		said += paintText(theme.Muted, theme.Panel, measure.label+" ")
		said += paintText(measure.ink, theme.Panel, measure.value)
	}
	return model.renderPanelLine(said, width)
}

// dashboardMeasure is one number of the summary: what it is called, what it reads, and how
// strongly it is drawn.
type dashboardMeasure struct {
	label string
	value string
	ink   color.Color
}

const (
	poorCacheHitRate = 0.90
	fairCacheHitRate = 0.99
)

const (
	fairReplicationLag = 5 * time.Second
	poorReplicationLag = time.Minute
)

// buildDashboardMeasures returns the numbers of the summary the server answered for. A rate
// is left out until there are two readings to measure it between.
func (model *Model) buildDashboardMeasures(overlay app.Overlay) []dashboardMeasure {
	theme := model.styles.Theme
	reading := overlay.Server
	if !reading.HasLoad {
		return nil
	}
	load := reading.Load
	measures := []dashboardMeasure{}

	span := reading.ReadAt.Sub(overlay.View.PreviousAt)
	rates := overlay.View.HasPrevious && load.HasCounters && overlay.View.Previous.HasCounters
	if rates {
		if rate, held := app.ResolveCounterRate(
			overlay.View.Previous.Transactions, load.Transactions, span); held {
			measures = append(measures, dashboardMeasure{
				label: "txn/s", value: core.FormatRate(rate), ink: theme.Text,
			})
		}
		if rate, held := app.ResolveCounterRate(
			overlay.View.Previous.WalBytes, load.WalBytes, span); held {
			measures = append(measures, dashboardMeasure{
				label: "wal", value: core.FormatByteRate(rate), ink: theme.Text,
			})
		}
	}
	if load.HasCacheHitRate {
		measures = append(measures, dashboardMeasure{
			label: "cache", value: core.FormatShare(load.CacheHitRate),
			ink: model.resolveCacheInk(load.CacheHitRate),
		})
	}
	if load.HasReplicationLag {
		measures = append(measures, dashboardMeasure{
			label: "lag", value: core.FormatLargestUnit(load.ReplicationLag),
			ink: model.resolveLagInk(load.ReplicationLag),
		})
	}
	if load.HasCounters {
		ink := theme.Text
		if load.TempFiles > 0 {
			ink = theme.Warning
		}
		measures = append(measures, dashboardMeasure{
			label: "tmp files", value: present.FormatCount(load.TempFiles), ink: ink,
		})
	}
	return measures
}

// resolveCacheInk returns the colour of the cache hit rate.
func (model *Model) resolveCacheInk(share float64) color.Color {
	theme := model.styles.Theme
	switch {
	case share < poorCacheHitRate:
		return theme.Error
	case share < fairCacheHitRate:
		return theme.Warning
	}
	return theme.Text
}

// resolveLagInk returns the colour of the replication lag.
func (model *Model) resolveLagInk(lag time.Duration) color.Color {
	theme := model.styles.Theme
	switch {
	case lag >= poorReplicationLag:
		return theme.Error
	case lag >= fairReplicationLag:
		return theme.Warning
	}
	return theme.Text
}

// resolveLoadInk returns the colour of the connection count, which reads as a warning near
// the limit and as a fault at it.
func (model *Model) resolveLoadInk(load db.ServerLoad) color.Color {
	theme := model.styles.Theme
	if load.MaxConnections <= 0 {
		return theme.Text
	}
	switch share := float64(load.Connections) / float64(load.MaxConnections); {
	case share >= 1:
		return theme.Error
	case share >= dashboardBusyShare:
		return theme.Warning
	}
	return theme.Text
}

// buildBlockingPanel returns the panel of sessions waiting for a lock, and none where
// no session waits.
func (model *Model) buildBlockingPanel(overlay app.Overlay, width int) []string {
	reading := overlay.Server
	if !reading.HasLocks || len(reading.Locks) == 0 {
		return nil
	}
	theme := model.styles.Theme
	folded := overlay.View.IsPanelFolded(app.PanelBlocking)

	heading := paintOn(theme.Panel, strings.Repeat(" ", rowPaddingLeft))
	heading += paintText(theme.Error, theme.Panel, model.readFoldMark(folded)+" blocking tree")
	if folded {
		heading += paintText(theme.Muted, theme.Panel, " · "+
			strconv.Itoa(app.CountBlockedSessions(reading.Locks))+" waiting")
		return []string{model.renderPanelLine(heading, width)}
	}

	nodes := app.BuildBlockingTree(reading.Locks)
	depths := make([]int, 0, len(nodes))
	for _, node := range nodes {
		depths = append(depths, node.Depth)
	}
	guides := present.BuildGuidesForDepths(depths)

	lines := []string{model.renderPanelLine(heading, width)}
	for at, node := range nodes {
		lines = append(lines, model.renderBlockingRow(node, guides[at], width))
	}
	return lines
}

// slowStatementRows is how many statements the panel draws.
const slowStatementRows = 5

// buildSlowPanel returns the panel of statements the server spends its time in.
func (model *Model) buildSlowPanel(overlay app.Overlay, width int) []string {
	reading := overlay.Server
	if !reading.HasSlow || len(reading.Slow) == 0 {
		return nil
	}
	theme := model.styles.Theme
	folded := overlay.View.IsPanelFolded(app.PanelSlow)

	heading := paintOn(theme.Panel, strings.Repeat(" ", rowPaddingLeft))
	heading += paintText(theme.Text, theme.Panel,
		model.readFoldMark(folded)+" slowest statements")
	heading += paintText(theme.Muted, theme.Panel, " · by mean time")
	if folded {
		return []string{model.renderPanelLine(heading, width)}
	}

	shown := reading.Slow
	if len(shown) > slowStatementRows {
		shown = shown[:slowStatementRows]
	}
	meanWidth := 0
	for _, held := range shown {
		meanWidth = max(meanWidth, present.MeasureText(core.FormatDuration(held.MeanTime)))
	}

	lines := []string{model.renderPanelLine(heading, width)}
	for _, held := range shown {
		lines = append(lines, model.renderSlowRow(held, meanWidth, width))
	}
	return lines
}

// renderSlowRow draws one statement the server spends its time in: how long it takes on
// average, how often it ran, and the statement itself.
func (model *Model) renderSlowRow(
	held db.StatementStat, meanWidth, width int,
) string {
	theme := model.styles.Theme
	mean := core.FormatDuration(held.MeanTime)
	calls := "×" + present.FormatCount(held.Calls)

	said := paintOn(theme.Panel, strings.Repeat(" ", rowPaddingLeft))
	said += paintText(theme.Text, theme.Panel,
		strings.Repeat(" ", max(meanWidth-present.MeasureText(mean), 0))+mean)
	said += paintText(theme.Muted, theme.Panel, " "+calls+" ")

	room := max(width-present.CardChrome-rowPaddingLeft-meanWidth-
		present.MeasureText(calls)-2-rowScrollbarWidth, 0)
	said += paintText(theme.Muted, theme.Panel, present.FitText(
		present.TruncateText(present.SafeText(
			core.CollapseWhitespace(held.Query)), room), room))
	return model.renderPanelLine(said, width)
}

// renderBlockingRow draws one session of the blocking tree: which session it is, what it is
// running, and how long it has been waiting or holding.
func (model *Model) renderBlockingRow(
	node app.BlockingNode, guide string, width int,
) string {
	theme := model.styles.Theme
	said := paintOn(theme.Panel, strings.Repeat(" ", rowPaddingLeft))
	said += paintText(theme.Muted, theme.Panel, guide)
	said += paintText(theme.Text, theme.Panel, strconv.FormatInt(node.PID, 10)+" ")

	trail := core.FormatClock(node.Elapsed)
	if node.Waiting {
		trail = "waiting " + trail
	} else if mode := core.FormatLockMode(node.Mode); mode != "" {
		trail = mode + " " + trail
	}
	trail = present.SafeText(" " + trail + " ")

	room := max(width-present.CardChrome-rowPaddingLeft-present.MeasureText(guide)-
		present.MeasureText(strconv.FormatInt(node.PID, 10))-1-
		present.MeasureText(trail)-rowScrollbarWidth, 0)
	said += paintText(theme.Muted, theme.Panel, present.FitText(
		present.TruncateText(present.SafeText(
			core.CollapseWhitespace(node.Query)), room), room))

	ink := theme.Warning
	if node.Waiting {
		ink = theme.Error
	}
	said += paintText(ink, theme.Panel, trail)
	return model.renderPanelLine(said, width)
}

// readFoldMark returns the glyph of a panel that is folded away, or of one that is open.
func (model *Model) readFoldMark(folded bool) string {
	if folded {
		return model.icons.Icon(cfg.IconFoldClosed)
	}
	return model.icons.Icon(cfg.IconFoldOpen)
}

// renderPanelLine pads a line of a panel to the width the rows of the list are drawn in.
func (model *Model) renderPanelLine(written string, width int) string {
	pad := paintOn(model.styles.Theme.Panel, " ")
	return pad + padStyledOn(written, width-4, model.styles.Theme.Panel) + pad + pad
}

// renderExport draws the file and the options an export is written with.
func (model *Model) renderExport(overlay app.Overlay, width int) string {
	fields := BuildExportFields(overlay)
	valueWidth := max(width-present.CardChrome-exportLabelWidth, 8)

	model.layout.formChoices = nil
	lines := make([]string, 0, len(fields)+3)
	for at, field := range fields {
		focused := at == overlay.Field
		marker := "  "
		labelStyle := model.styles.Muted()
		if focused {
			marker = present.FitText(model.icons.Icon(cfg.IconField), fieldMarkerWidth)
			labelStyle = model.styles.Accent()
		}

		value := field.Value
		written := model.styles.Muted().Render(present.TruncateText(value, valueWidth))
		switch {
		case len(field.Choices) > 0:
			written = model.renderChoiceField(value, valueWidth, at,
				cardBodyRow+at, cardBodyColumn+exportLabelWidth, focused)
		case focused:
			written = model.renderField(
				app.NewEditorBuffer(value, len(value)), valueWidth, FieldLook{
					Ground: model.styles.Theme.Header, Ink: model.styles.Theme.Text,
					Focused: true, Placeholder: field.Label,
				})
		}
		lines = append(lines, labelStyle.Render(
			marker+fitFieldLabel(field.Label, exportLabelWidth-present.MeasureText(marker)))+written)
	}

	// The problem line is always counted, so the card keeps its height.
	lines = append(lines, model.styles.Error().Render(
		present.TruncateText(FindExportProblem(overlay), width-4)))

	keys := model.sayKeys().name("↑↓", "field").name("← →", "change").
		bind(cfg.ScopeDialog, ActionWriteExport, "write").
		bind(cfg.ScopeDialog, ActionClose, "cancel")
	// The keys are cut rather than wrapped here, because the card keeps one row for them.
	said := present.TruncateText(keys.buildText(), width-4)
	model.recordCardBody()
	lines = model.appendCardKeyRow(lines, keys, said, cardBodyRow, cardBodyColumn)
	model.rememberCardKeys(keys)
	model.layout.formRows = rowsHit{
		top: model.layout.cardBodyTop, count: len(fields),
		from: model.layout.cardBodyLeft - 1, to: model.layout.cardBodyLeft + width - 4,
	}
	return model.renderCard(
		" export "+present.FormatRowCount(int64(overlay.Export.RowCount))+" ",
		width, lines, plainCard)
}

// exportLabelWidth is the column the mark and the name of a field of a form share.
const exportLabelWidth = 12

// fitFieldLabel holds the name of a field to the column it is given. A name too long for the
// column keeps the words that fit, because the row draws one row of the label.
func fitFieldLabel(written string, width int) string {
	wrapped := present.WrapWords(written, width)
	if len(wrapped) == 0 {
		return present.FitText(written, width)
	}
	return present.FitText(wrapped[0], width)
}

// promptHints name what each prompt does, which the field alone cannot show.
var promptHints = map[app.PromptKind]string{
	app.PromptSearch: "hides the rows on screen that hold nothing matching · " +
		"the server is not asked",
	app.PromptWhere:      "filters the result · your query is not changed",
	app.PromptGoToColumn: "moves the cursor to the first column whose name matches",
	app.PromptTabName:    "written as a comment on the first line of the query",
	app.PromptFind:       "marks every match · F3 steps to the next",
	app.PromptReplace:    "writes this in place of every match, in one step",
}

// drawsPromptBar is true for a prompt that opens a field at the foot of a pane. Only the one
// that names a saved query is a card, because it is asked from the palette and belongs to no
// pane.
func drawsPromptBar(overlay app.Overlay) bool {
	return overlay.Kind == app.OverlayPrompt && overlay.Prompt != app.PromptSaveName
}

// findPromptBar returns the prompt to draw at the foot of a pane, and whether there is one.
func findPromptBar(connection *app.Connection, kinds ...app.PromptKind) (app.Overlay, bool) {
	overlay := connection.Overlay
	if !drawsPromptBar(overlay) {
		return app.Overlay{}, false
	}
	if slices.Contains(kinds, overlay.Prompt) {
		return overlay, true
	}
	return app.Overlay{}, false
}

// renderPromptBar draws the two rows a prompt takes at the foot of a pane: the field, and what
// answering it does.
func (model *Model) renderPromptBar(overlay app.Overlay, width int) []string {
	theme := model.styles.Theme
	label := paintText(theme.Accent, theme.Header, overlay.Title+" ")
	room := max(width-1-measureStyledWidth(label), 1)
	field := model.renderField(overlay.Draft, room, FieldLook{
		Ground: theme.Header, Ink: theme.Text, Focused: true,
	})

	// The hint is cut where the pane ends, with nothing to mark the cut, because the keys it
	// names are on the bar below as well.
	hint := promptHints[overlay.Prompt] + " · ↵ apply · Esc cancel"
	// Finding and replacing is one key, so the find field is where the second half is offered.
	if overlay.Prompt == app.PromptFind {
		if chord := model.registry.FormatActionChordCompact(
			cfg.ScopeDialog, ActionReplaceInStatement); chord != "" {
			hint = promptHints[overlay.Prompt] + " · " + chord +
				" replace · ↵ apply · Esc cancel"
		}
	}
	return []string{
		paintOn(theme.Header, " ") + label + field,
		" " + model.styles.Muted().Render(truncateCells(hint, width-1)),
	}
}

// renderPrompt draws a one-line field, with what it is for under it.
func (model *Model) renderPrompt(overlay app.Overlay, width int) string {
	inner := width - 4
	keys := model.sayKeys().
		bind(cfg.ScopeList, ActionChooseRow, "applies").
		bind(cfg.ScopeDialog, ActionClose, "cancel")
	said := present.TruncateText(keys.buildText(), inner)
	lines := []string{
		model.renderField(overlay.Draft, inner, FieldLook{
			Ground: model.styles.Theme.Header, Ink: model.styles.Theme.Text,
			Focused: true, Placeholder: overlay.Title,
		}),
		model.styles.Muted().Render(present.TruncateText(overlay.Hint, inner)),
	}
	lines = model.appendCardKeyRow(lines, keys, said, cardBodyRow, cardBodyColumn)
	model.rememberCardKeys(keys)
	return model.renderCard(" "+overlay.Title+" ", width, lines, plainCard)
}

// wrapErrorText breaks a block of text into lines that fit the width, in the ink of a fault.
func (model *Model) wrapErrorText(text string, width int) []string {
	written := []string{}
	for _, line := range model.wrapText(text, width) {
		written = append(written, model.styles.Error().Render(line))
	}
	return written
}

// wrapText breaks a block of text into the lines it takes at a width. A line break of the
// text is kept, and each line between the breaks is wrapped on its own.
func (model *Model) wrapText(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	written := []string{}
	for line := range strings.SplitSeq(text, "\n") {
		if line == "" {
			written = append(written, "")
			continue
		}
		written = append(written, present.WrapWords(line, width)...)
	}
	return written
}
