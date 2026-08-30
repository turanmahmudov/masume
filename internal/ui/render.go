package ui

import (
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// titleBarRows is the row the title bar takes over every screen. A renderer that places
// something on the frame of a screen counts from under it.
const titleBarRows = 1

// render draws the frame: the title bar, the screen, and the status bar under it.
//
// Drawing writes into the state as well as reading it: it clamps the offset of a view to what
// it holds, and records where each button, row and bar landed. Only the draw knows the layout
// at this width, and a press is answered against the frame the user is looking at, so both are
// kept here rather than worked out a second time.
func (model *Model) render() string {
	model.forgetClosedTabs()
	// Every key a renderer draws as a word, and every scroll bar it draws, it records here,
	// so the frame is walked once and a press finds what it landed on.
	model.layout.buttons = nil
	model.layout.scrollbars = nil
	// The keys of the card on show are learnt while the card is drawn, which is before the
	// status bar under it is.
	model.cardKeys = nil

	// The rows the title bar and the status bar take, whatever the screen is.
	body := max(model.height-2, 1)

	var middle []string
	switch model.screen {
	case ScreenWorking:
		middle = model.renderWorkspace(body)
	case ScreenPickingProfile:
		middle = model.styles.CenterRowsOn(model.renderPicker(), model.width, body)
	case ScreenPromptingPassword:
		middle = model.styles.CenterRowsOn(model.renderPassword(), model.width, body)
	case ScreenEditingConnection:
		middle = model.styles.CenterRowsOn(model.renderForm(), model.width, body)
	case ScreenConnecting:
		middle = model.styles.CenterRowsOn(model.renderConnecting(), model.width, body)
	}

	// A question a screen without a connection asks is drawn over it.
	if model.confirm != nil {
		card := model.renderCard(model.confirm.Title,
			present.ResolveCardWidth(64, 36, model.width), []string{
				model.confirm.Body, "",
				model.styles.Muted().Render(model.describeConfirmKeys()),
			}, destructiveCard)
		middle = model.styles.CenterRowsOn(card, model.width, body)
	}

	rows := make([]string, 0, len(middle)+2)
	rows = append(rows, model.renderTitleBar())
	rows = append(rows, middle...)
	rows = append(rows, model.renderScreenStatusBar())
	return strings.Join(rows, "\n")
}

// renderConnecting draws the line that says which server the client is waiting for.
func (model *Model) renderConnecting() string {
	return model.styles.Accent().Render(
		spinnerFrame(model.spinnerAt) + " connecting to " + model.picker.pending.Name)
}

// spinnerFrames is the wheel drawn while something runs.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func spinnerFrame(at int) string {
	return spinnerFrames[at%len(spinnerFrames)]
}

// showSecondsFrom is the wait below which the seconds are not shown.
const showSecondsFrom = time.Second

// describeElapsed writes how long a wait has run, and nothing for a wait that just began.
func describeElapsed(since time.Time) string {
	if since.IsZero() {
		return ""
	}
	elapsed := time.Since(since)
	if elapsed < showSecondsFrom {
		return ""
	}
	return " " + strconv.Itoa(int(elapsed.Seconds())) + "s"
}

// renderThinkingLine draws every wait the client draws: a running statement, a catalog read, a
// connection, or the assistant. A reader cannot tell one from another on the screen alone, so
// all of them turn the same wheel. The dots at the end say the work goes on, and a label that
// has them keeps its own.
func (model *Model) renderThinkingLine(label string, since time.Time, ground color.Color) string {
	theme := model.styles.Theme
	said := label
	if !strings.HasSuffix(said, "…") {
		said += "…"
	}
	return paintText(theme.Accent, ground, spinnerFrame(model.spinnerAt)+" ") +
		paintText(theme.Muted, ground, said+describeElapsed(since))
}

// describeConfirmKeys names the keys a question returns to.
func (model *Model) describeConfirmKeys() string {
	return model.registry.FormatActionChords(cfg.ScopeDialog, ActionAnswerYes) + " yes · " +
		model.registry.FormatActionChords(cfg.ScopeDialog, ActionAnswerNo) + " no"
}

// The keys the title bar names on the right, so the palette, the help and the chat stay in
// view. The chat is shown even without a model key: a press then reports the config file, and
// a missing chip would look like an app without a chat.
var titleBarShortcuts = []struct {
	id    ActionID
	label string
	// The glyph the key carries, or nothing for a key that needs none.
	icon cfg.IconKind
}{
	{id: ActionShowPalette, label: "palette"},
	{id: ActionShowHelp, label: "help"},
	{id: ActionShowAiChat, label: "ask ai", icon: cfg.IconAi},
}

// The logo of the client, and the run of blanks that holds the name apart from what the bar
// reports beside it. The logo is the name written the way it is read, the squares of a grid,
// so it is the mark of the client itself rather than a glyph a config file chooses.
const (
	logoGlyph = "升目"
	logoGap   = "   "
)

// renderTitleBar draws the top bar: the active connection on the left, the global keys on the
// right.
func (model *Model) renderTitleBar() string {
	theme := model.styles.Theme
	connection := model.Active()

	// A production connection colours the whole bar, and not only the word. The word is
	// read by a user who looks for it, but the bar is seen from across the room.
	onProduction := connection != nil &&
		connection.Profile().Environment == cfg.EnvironmentProd
	ground := theme.Header
	if onProduction {
		ground = theme.EnvProd
	}

	// Every colour of the bar becomes one ink while it is a warning. With its own accents
	// the bar would look like parts on a red ground, not like one warning.
	lead := theme.Accent
	follow := theme.Muted
	if onProduction {
		lead = model.styles.InkOn(theme.EnvProd)
		follow = lead
	}
	logoInk := theme.AccentAlt
	if onProduction {
		logoInk = lead
	}
	name := paintBoldText(logoInk, ground, logoGlyph+" ") +
		paintBoldText(lead, ground, "masume")
	if connection == nil {
		return model.styles.RenderStrip(ground, model.width, name, "")
	}

	profile := connection.Profile()
	// A state mark keeps its own colour, unless the whole bar is one warning already.
	resolveMarkInk := func(held color.Color) color.Color {
		if onProduction {
			return model.styles.InkOn(theme.EnvProd)
		}
		return held
	}

	var said strings.Builder
	said.WriteString(name)
	writeTextOn(&said, follow, ground, logoGap)
	writeTextOn(&said, resolveMarkInk(model.styles.EnvironmentColor(profile.Environment)),
		ground, string(profile.Environment))
	if profile.AccessMode == cfg.AccessReadOnly {
		writeTextOn(&said, follow, ground, hintSeparator)
		writeTextOn(&said, resolveMarkInk(theme.Warning), ground, "RO")
	}
	writeTextOn(&said, follow, ground, hintSeparator+string(profile.Engine)+" "+
		present.SafeText(connection.Session.Describe().ServerVersion))

	if !connection.Autocommit {
		writeTextOn(&said, resolveMarkInk(theme.Warning), ground, hintSeparator+"manual")
	}
	switch connection.Health {
	case app.HealthDown:
		writeTextOn(&said, resolveMarkInk(theme.Danger), ground,
			hintSeparator+"not answering")
	case app.HealthReconnecting:
		writeTextOn(&said, resolveMarkInk(theme.Warning), ground,
			hintSeparator+"reconnecting")
	}
	switch connection.Session.ReadTransactionState() {
	case db.TransactionOpen:
		writeTextOn(&said, resolveMarkInk(theme.Warning), ground, hintSeparator+"tx open")
	case db.TransactionFailed:
		writeTextOn(&said, resolveMarkInk(theme.Error), ground, hintSeparator+"tx failed")
	}

	// The keys on the right are buttons as well, while no card is open. A card returns the
	// keys of its own, so a key of the bar behind it would look live and do nothing. They
	// are drawn a step back while one is open, in the quiet ink and not the faint one,
	// which a reader cannot make out at all.
	keyInk, keyFollow := lead, follow
	if connection.Overlay.IsOpen() {
		keyInk, keyFollow = theme.Muted, theme.Muted
	}

	// The keys are held against the right corner, so the cells each one covers are counted
	// back from there. Every part is measured from the text that was written, so the count
	// and the row can never drift apart.
	var keys strings.Builder
	covered := 0
	boxes := []buttonHit{}
	writeKey := func(ink color.Color, text string) {
		writeTextOn(&keys, ink, ground, text)
		covered += present.MeasureText(text)
	}
	for _, shortcut := range titleBarShortcuts {
		chord := model.registry.FormatActionChordCompact(cfg.ScopeGlobal, shortcut.id)
		if chord == "" {
			continue
		}
		if keys.Len() > 0 {
			writeKey(keyFollow, hintSeparator)
		}
		from := covered
		writeKey(keyInk, chord)
		// A key that names a glyph carries it between the chord and what it does, in the
		// colour of what it stands for, unless the bar is drawn a step back already.
		if glyph := model.icons.Icon(shortcut.icon); shortcut.icon != "" && glyph != "" {
			glyphInk := model.styles.IconColor(shortcut.icon)
			if onProduction || connection.Overlay.IsOpen() {
				glyphInk = keyInk
			}
			writeKey(glyphInk, " "+glyph)
		}
		writeKey(keyFollow, " "+shortcut.label)
		boxes = append(boxes, buttonHit{
			row: 0, from: from, to: covered - 1,
			scope: cfg.ScopeGlobal, action: shortcut.id,
		})
	}

	model.layout.titleRow = 0
	// A bar too narrow for both halves cuts the left one, and the right one stands where it
	// was counted from, so the boxes hold only where the whole row was drawn.
	left := model.width - 1 - covered
	if covered > 0 && left > present.MeasureText("masume") && !connection.Overlay.IsOpen() {
		for _, box := range boxes {
			box.from += left
			box.to += left
			box.keyTo = box.to
			model.layout.buttons = append(model.layout.buttons, box)
		}
	}

	return model.styles.RenderStrip(ground, model.width, said.String(), keys.String())
}

// renderScreenStatusBar draws the bar under a screen that has no connection.
func (model *Model) renderScreenStatusBar() string {
	if model.screen == ScreenWorking {
		return model.renderWorkspaceStatusBar()
	}

	var hints []Hint
	switch model.screen {
	case ScreenConnecting:
		hints = model.registry.BuildConnectingHints(model.holdsSelection())
	case ScreenEditingConnection, ScreenPromptingPassword:
		hints = model.registry.BuildCardScreenHints(model.holdsSelection())
	default:
		hints = model.registry.BuildPickerHints(model.holdsSelection())
	}

	// A copy the reader just made is reported first, because it returns what they did.
	if model.copied != "" {
		return model.renderStatusBar(hints, model.copied, app.NoticeInfo)
	}
	// A screen without a connection has no notice of its own, so the count of the faults in
	// the config goes here. The palette names each one.
	return model.renderStatusBar(hints, model.describeConfigProblems(), app.NoticeError)
}

// describeConfigProblems writes the line for the status bar, and nothing where the config
// file and the theme files gave no fault.
func (model *Model) describeConfigProblems() string {
	if len(model.problems) == 0 {
		return ""
	}
	return model.writeProblemSign() +
		present.FormatCountOf(int64(len(model.problems)), "problem", "problems") +
		" in your config"
}

// renderStatusBar draws the bottom bar: the keys on the left, the report on the right.
func (model *Model) renderStatusBar(hints []Hint, message string, tone app.NoticeTone) string {
	theme := model.styles.Theme
	// The blank column on each side of the message, and the gap before the keys.
	room := model.width - present.MeasureText(message) - 4

	separator := paintText(theme.Faint, theme.Header, hintSeparator)

	// The bar is the last row of the frame, and each key of it is a button: the cells it
	// covers are recorded so a press runs the same action the key does.
	model.layout.hintRow = model.height - 1

	var written strings.Builder
	used := 1
	for _, hint := range FitHints(hints, room) {
		keyInk, labelInk := theme.Accent, theme.Muted
		if hint.Standing {
			keyInk, labelInk = theme.Muted, theme.Faint
		}
		if written.Len() > 0 {
			written.WriteString(separator)
			used += present.MeasureText(hintSeparator)
		}
		from := used
		writeTextOn(&written, keyInk, theme.Header, hint.Key)
		used += present.MeasureText(hint.Key)
		keyTo := used - 1
		if hint.Label != "" {
			writeTextOn(&written, labelInk, theme.Header, " "+hint.Label)
			used += present.MeasureText(hint.Label) + 1
		}
		// A key that quits is never a button: a press meant for the row under it would
		// close the client. Only the workspace returns a press on a key of the bar, so a
		// screen that has no connection records none.
		if hint.Standing || hint.Action == "" || model.screen != ScreenWorking {
			continue
		}
		model.layout.buttons = append(model.layout.buttons, buttonHit{
			row: model.layout.hintRow, from: from, to: used - 1, keyTo: keyTo,
			scope: hint.Scope, action: hint.Action, second: hint.Second,
		})
	}

	messageColor := theme.Muted
	switch tone {
	case app.NoticeError:
		messageColor = theme.Error
	case app.NoticeActive:
		messageColor = theme.Accent
	}
	right := ""
	if message != "" {
		right = paintText(messageColor, theme.Header, message)
	}

	return model.styles.RenderStrip(theme.Header, model.width, written.String(), right)
}

// renderWorkspaceStatusBar draws the bar under the workspace, with the keys of the pane that
// holds the caret.
func (model *Model) renderWorkspaceStatusBar() string {
	connection := model.Active()
	if connection == nil {
		return model.renderStatusBar(nil, "", app.NoticeInfo)
	}
	tab := connection.Active()

	// A card on show returns the keys the bar would name, so the bar names the keys of the
	// card instead. Without this the bar offers what the pane behind the card returns, and
	// neither a press nor a key reaches it.
	if connection.Overlay.IsOpen() {
		message, tone := model.describeStatus(connection, tab)
		return model.renderStatusBar(
			addCopyOrQuit(model.cardKeys.buildHints(), model.holdsSelection()), message, tone)
	}

	rows := model.treeRows(connection)
	var treeRow *present.TreeRow
	if tab.Focus == app.PaneSidebar && len(rows) > 0 {
		held := rows[clamp(connection.Tree.Cursor, len(rows))]
		treeRow = &held
	}

	active := tab.Results.Active()
	hasResult := active != nil && active.State.Kind == app.QuerySucceeded
	running := active != nil && active.State.Kind == app.QueryRunning
	failed := active != nil && active.State.Kind == app.QueryFailed

	hints := model.registry.BuildHints(HintContext{
		Pane: tab.Focus, Capabilities: connection.Session.Capabilities(),
		TabKind: tab.Kind, View: tab.View, Views: tab.Views(connection.Session),
		HasResult: hasResult, Connections: model.connections.count(),
		HasSelection: model.holdsSelection(), SidebarVisible: connection.SidebarVisible,
		Rewritten: tab.HasRewrite(), FilterSteps: len(tab.Filter),
		Staged:       core.CountChanges(tab.Pending),
		CanFetchMore: tab.Results.CanFetchMore(),
		CanCountRows: tab.Results.CanCountRows() &&
			(active == nil || !active.HasTotalRows),
		Running: running, QueryFailed: failed, TreeRow: treeRow,
	})

	// The sort and the filter are in the banner above the grid, and a failed query is
	// reported in the pane that would hold its rows.
	message, tone := model.describeStatus(connection, tab)
	return model.renderStatusBar(hints, message, tone)
}

// describeStatus returns what the bar writes on the right, and how strongly.
func (model *Model) describeStatus(
	connection *app.Connection, tab *app.Tab,
) (string, app.NoticeTone) {
	// A new report is more important than the staged work. A report carries what the
	// server said, so it is made safe to draw.
	// A report that has to be acted on carries its mark, so what the bar says is read by
	// its shape before it is read as words.
	warning := model.icons.Prefix(cfg.IconNote)
	if connection.Notice != nil {
		written := present.SafeText(connection.Notice.Text)
		if connection.Notice.Tone == app.NoticeError {
			return model.writeProblemSign() + written, app.NoticeError
		}
		return written, app.NoticeActive
	}

	staged := core.CountChanges(tab.Pending)
	if staged > 0 {
		return model.icons.Prefix(cfg.IconDot) + strconv.Itoa(staged) + " staged", app.NoticeActive
	}

	switch connection.Session.ReadTransactionState() {
	case db.TransactionOpen:
		return warning + "a transaction is open", app.NoticeInfo
	case db.TransactionFailed:
		return model.writeProblemSign() + "the transaction failed: roll it back",
			app.NoticeError
	}
	if !connection.Autocommit {
		return warning + "autocommit is off", app.NoticeInfo
	}
	return "", app.NoticeInfo
}
