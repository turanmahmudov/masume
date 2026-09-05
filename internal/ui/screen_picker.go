package ui

import (
	"image/color"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/present"
)

// The widths of one row of the picker. Each part is measured and cut to fit, so a row that
// is too wide does not clip: its parts share the width.
const (
	widestPickerCard    = 96
	narrowestPickerCard = 48
	// The password card holds one field, so it is narrower than the list.
	widestPasswordCard    = 60
	narrowestPasswordCard = 32
	pickerNameWidth       = 24
	pickerEnvWidth        = 4
	// `ro`, or two spaces for a connection that can be written to.
	pickerModeWidth = 2
	// `project`, and the blank after it. The column stands empty where no connection
	// comes from a project file.
	pickerSourceWidth = 8
	// The border, the padding of the card, and the padding of a row.
	pickerChrome = 6
	pickerGap    = 1
	// pickerCardChrome is the border, the blank row inside it, the blank row over the
	// keys, and the row of keys.
	pickerCardChrome = 6
)

// pickerActions are the actions the profile picker handles. The dialog scope binds `n` to
// `answer-no` as well as to `new-connection`, so the screen names the ones it takes.
var pickerActions = append(collectScopeActions(cfg.ScopeList),
	ActionClose, ActionNewConnection, ActionEditConnection, ActionDeleteConnection)

// readPickerKey returns what one press does in the profile picker.
func (model *Model) readPickerKey(key tea.Key) (tea.Model, tea.Cmd) {
	// Escape belongs to no action. It closes the picker and goes back to the connection
	// that is open.
	if key.Code == tea.KeyEscape {
		if model.connections.count() > 0 {
			model.screen = ScreenWorking
		}
		return model, nil
	}

	match, matched := model.keymap.MatchOnly(key, pickerActions, cfg.ScopeDialog, cfg.ScopeList)
	if !matched {
		return model, nil
	}
	return model.runPickerAction(match)
}

// runPickerAction runs one action of the connection picker, whether a key or a press asked
// for it.
func (model *Model) runPickerAction(match Match) (tea.Model, tea.Cmd) {
	count := len(model.profiles)
	switch match.Action {
	case ActionCursorUp:
		model.picker.step(-1, count)
	case ActionCursorDown:
		model.picker.step(1, count)
	case ActionCursorPageUp:
		model.picker.page(-listPage, count)
	case ActionCursorPageDown:
		model.picker.page(listPage, count)
	case ActionCursorFirstRow:
		model.picker.focus(0, count)
	case ActionCursorLastRow:
		model.picker.focus(count-1, count)
	case ActionChooseRow:
		if profile, found := model.pickedProfile(); found {
			return model.chooseProfile(profile)
		}
	case ActionNewConnection:
		model.form = NewFormState(cfg.Profile{}, false, model.secretStoreNames())
		model.screen = ScreenEditingConnection
	case ActionEditConnection:
		if profile, found := model.pickedProfile(); found {
			model.form = NewFormState(profile, true, model.secretStoreNames())
			model.screen = ScreenEditingConnection
		}
	case ActionDeleteConnection:
		if profile, found := model.pickedProfile(); found {
			return model.askDeleteProfile(profile)
		}
	}
	return model, nil
}

// listPage is how many rows a page key moves in a list.
const listPage = 10

func wrap(index, count int) int {
	if count <= 0 {
		return 0
	}
	return ((index % count) + count) % count
}

func clamp(index, count int) int {
	if count <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index > count-1 {
		return count - 1
	}
	return index
}

// pickedProfile returns the profile the cursor stands on.
func (model *Model) pickedProfile() (cfg.Profile, bool) {
	return model.picker.pick(model.profiles)
}

// renderPicker draws the connections of the config file and of the project file, one row
// each. The screen draws its own rows, because a row holds parts that each keep their own
// width.
func (model *Model) renderPicker() string {
	theme := model.styles.Theme
	cardWidth := present.ResolveCardWidth(widestPickerCard, narrowestPickerCard, model.width)
	// The source column stands empty where no connection comes from a project file, so a
	// user without one loses no room to it.
	sourceWidth := 0
	if slices.ContainsFunc(model.profiles, func(profile cfg.Profile) bool {
		return profile.ProjectFile != ""
	}) {
		sourceWidth = pickerSourceWidth
	}
	targetWidth := max(cardWidth-pickerChrome-pickerNameWidth-pickerEnvWidth-
		pickerModeWidth-sourceWidth-pickerGap*3, 12)

	lines := []string{}
	if len(model.profiles) == 0 {
		lines = append(lines, model.styles.Muted().Render(
			"No connections found in ~/.config/masume/config.toml"))
	}

	// Where the rows land on the screen, so a press opens the row it looks like. The card
	// stands in the middle of everything under the title bar, with a blank row inside its
	// border.
	cardRows := len(model.profiles) + pickerCardChrome
	if model.picker.problem != "" {
		cardRows += 2
	}
	if model.connections.count() > 0 {
		cardRows++
	}
	if model.project.Path != "" {
		cardRows++
	}
	left := halfRoundedUp(model.width - cardWidth)
	// The card stands under the title bar, which takes the first row of the screen.
	cardTop := titleBarRows + halfRoundedUp(model.height-2-cardRows)
	model.layout.pickerRows = rowsHit{
		top:   cardTop + cardBodyRow,
		count: len(model.profiles),
		from:  left + 1, to: left + cardWidth - 2,
	}

	for index, profile := range model.profiles {
		selected := index == model.picker.cursor
		name := present.FitText(profile.Name, pickerNameWidth)
		environment := present.FitText(string(profile.Environment), pickerEnvWidth)
		mode := "  "
		if profile.AccessMode == cfg.AccessReadOnly {
			mode = "ro"
		}
		source := ""
		if profile.ProjectFile != "" {
			source = "project"
		}
		target := present.TruncateText(cfg.DescribeProfileTarget(profile), targetWidth)

		row := lipgloss.NewStyle().Background(theme.Panel)
		nameStyle := model.styles.Ink().Background(theme.Panel)
		envStyle := lipgloss.NewStyle().
			Foreground(model.styles.EnvironmentColor(profile.Environment)).Background(theme.Panel)
		modeStyle := model.styles.Muted().Background(theme.Panel)
		targetStyle := model.styles.Faint().Background(theme.Panel)

		if selected {
			row = row.Background(theme.Accent)
			ink := lipgloss.NewStyle().Foreground(theme.OnAccent).Background(theme.Accent)
			nameStyle, envStyle, modeStyle, targetStyle = ink, ink, ink, ink
		}

		// One line, not five columns, which would share the width and cut every name short.
		written := nameStyle.Render(name+" ") + envStyle.Render(environment+" ") +
			modeStyle.Render(mode+" ")
		if sourceWidth > 0 {
			written += modeStyle.Render(present.FitText(source, sourceWidth))
		}
		written += targetStyle.Render(target)
		lines = append(lines, row.Width(cardWidth-2).Render(" "+written))
	}

	if model.picker.problem != "" {
		lines = append(lines, "", model.styles.Error().Render(
			present.TruncateText(model.picker.problem, cardWidth-4)))
	}

	keys := model.sayKeys().
		name("Enter", "or double click connects").
		bind(cfg.ScopeDialog, ActionNewConnection, "new").
		bind(cfg.ScopeDialog, ActionEditConnection, "edit").
		bind(cfg.ScopeDialog, ActionDeleteConnection, "delete")
	// The keys are cut rather than wrapped, because the card keeps one row for them.
	said := present.TruncateText(keys.buildText(), cardWidth-4)
	lines = model.appendCardKeyRow(
		lines, keys, said, cardTop+cardBodyRow, left+cardBodyColumn)
	if model.project.Path != "" {
		lines = append(lines, model.styles.Muted().Render(
			present.TruncateText("project file "+model.project.Path, cardWidth-4)))
	}
	if model.connections.count() > 0 {
		lines = append(lines, model.styles.Muted().Render("Esc back to the open connection"))
	}

	return model.renderCard(" connections ", cardWidth, lines, plainCard)
}

// renderCard draws a card with its title on the top border.
func (model *Model) renderCard(title string, width int, lines []string, destructive bool) string {
	inner := max(width-4, 1)
	padded := make([]string, 0, len(lines)+2)
	padded = append(padded, "")
	for _, line := range lines {
		padded = append(padded, " "+padStyledOn(line, inner, model.styles.Theme.Panel)+" ")
	}
	padded = append(padded, "")

	return model.styles.RenderBox(BoxOptions{
		Width: width, Height: len(padded) + 2, Title: title,
		Focused: true, Destructive: destructive, Lines: padded,
	})
}

// renderPassword draws the field a password is typed into, and the profile it is for.
func (model *Model) renderPassword() string {
	theme := model.styles.Theme
	cardWidth := present.ResolveCardWidth(
		widestPasswordCard, narrowestPasswordCard, model.width)
	profile := model.picker.pending
	keys := model.sayKeys().name("Enter", "connect").name("Esc", "cancel")

	lines := []string{
		model.styles.Muted().Render("connecting to ") +
			model.styles.Ink().Render(profile.Name) +
			model.styles.Muted().Render(" · ") +
			paintText(model.styles.EnvironmentColor(profile.Environment), nil, string(profile.Environment)),
		model.styles.Muted().Render(present.TruncateText(
			cfg.DescribeProfileTarget(profile), cardWidth-4)),
		"",
		model.renderField(model.picker.password, cardWidth-4, FieldLook{
			Ground: theme.Background, Ink: theme.Text,
			Masked: true, Focused: true, Placeholder: "password",
		}),
	}
	if model.picker.offersKeyring() {
		keys = keys.name("Tab", "keyring")
		lines = append(lines, "", model.renderKeyringBox(cardWidth))
	}
	lines = append(lines, "", model.styles.Muted().Render(
		present.TruncateText(keys.buildText(), cardWidth-4)))
	return model.renderCard(" password ", cardWidth, lines, plainCard)
}

// renderKeyringBox draws the box that keeps the typed password in the keyring of the
// operating system.
func (model *Model) renderKeyringBox(cardWidth int) string {
	mark := "[ ]"
	style := model.styles.Muted()
	if model.picker.keepInKeyring {
		mark = "[x]"
		style = model.styles.Ink()
	}
	return style.Render(present.TruncateText(
		mark+" remember in the keyring", cardWidth-4))
}

// FieldLook says how one field of a form or a card is drawn.
type FieldLook struct {
	// Ground is what the field stands on. Only the field being typed into takes a ground
	// of its own, so a row of fields does not read as one dark block.
	Ground color.Color
	// Ink is the colour of the value. A field without the caret is quieter.
	Ink color.Color
	// Masked writes a dot per character, for a password.
	Masked bool
	// Focused is true for the field the caret is in, which draws the caret.
	Focused bool
	// Placeholder stands where the field is empty and does not hold the caret.
	Placeholder string
	// KeepsPlaceholder draws the placeholder in a field that is empty and holds the caret,
	// which is what the question of the chat does.
	KeepsPlaceholder bool
}

// renderDraftRows draws a draft that runs over several lines, one string per row of the field.
// A field is one row of a card, so a renderer that answered one string with a line break in it
// would put every line after the first outside the card. The rows follow the caret, so the line
// being typed is always among them, and each row is filled out to the width on the ground of the
// field.
func (model *Model) renderDraftRows(
	buffer *app.EditorBuffer, width, rows int, look FieldLook,
) []string {
	if rows < 1 {
		rows = 1
	}
	if buffer.Text == "" {
		return append([]string{model.renderField(buffer, width, look)},
			buildBlankRows(look.Ground, width, rows-1)...)
	}

	held := buffer.Lines()
	caretLine, caretColumn := buffer.CaretPosition()
	offset := scrollTo(caretLine, 0, rows, len(held))

	drawn := make([]string, 0, rows)
	for at := offset; at < len(held) && len(drawn) < rows; at++ {
		drawn = append(drawn, model.renderDraftRow(
			held[at], width, look, at == caretLine, caretColumn))
	}
	return append(drawn, buildBlankRows(look.Ground, width, rows-len(drawn))...)
}

// renderDraftRow draws one line of a draft, with the caret on its cell where it stands on this
// line.
func (model *Model) renderDraftRow(
	line string, width int, look FieldLook, holdsCaret bool, caretColumn int,
) string {
	theme := model.styles.Theme
	// The field holds what the server sent, so what is drawn from it is made safe first.
	// The buffer keeps the value as it stands, because a save writes it back.
	line = present.SafeText(line)
	if !holdsCaret || !look.Focused {
		return padStyledOn(paintText(look.Ink, look.Ground,
			present.TruncateText(line, width)), width, look.Ground)
	}

	head, under, tail := splitAtCaret(line, len([]rune(line[:min(caretColumn, len(line))])))
	return padStyledOn(truncateStyled(paintText(look.Ink, look.Ground, head)+
		paintText(theme.OnAccent, theme.Accent, under)+
		paintText(look.Ink, look.Ground, tail), width), width, look.Ground)
}

// splitAtCaret returns the text before the caret, the character under it and the text after
// it. A caret at the end of the line stands on a blank, so the field always draws one.
func splitAtCaret(line string, caret int) (string, string, string) {
	runes := []rune(line)
	head := string(runes[:min(max(caret, 0), len(runes))])
	if caret < 0 || caret >= len(runes) {
		return head, " ", ""
	}
	return head, string(runes[caret]), string(runes[caret+1:])
}

// buildBlankRows returns the rows of a field that hold no line of the draft.
func buildBlankRows(ground color.Color, width, rows int) []string {
	blanks := make([]string, 0, max(0, rows))
	for range rows {
		blanks = append(blanks, paintBlanks(ground, width))
	}
	return blanks
}

// renderField draws a one-line field, with the caret where it stands.
func (model *Model) renderField(
	buffer *app.EditorBuffer, width int, look FieldLook,
) string {
	theme := model.styles.Theme
	style := lipgloss.NewStyle().Background(look.Ground).Foreground(look.Ink)

	written := buffer.Text
	if look.Masked {
		written = strings.Repeat("•", len([]rune(written)))
	}
	if written == "" && (!look.Focused || look.KeepsPlaceholder) {
		placeholder := model.styles.Muted().Background(look.Ground).
			Render(present.TruncateText(look.Placeholder, width))
		if !look.Focused {
			return style.Width(width).Render(placeholder)
		}
		// The caret stands on the first cell of the placeholder, because that is where a
		// typed character lands.
		return style.Width(width).Render(paintCaretOverStart(
			placeholder, theme.Accent, theme.OnAccent))
	}
	if !look.Focused {
		return style.Width(width).Render(present.TruncateText(written, width))
	}

	// The caret is drawn on the cell it stands on, so the field needs no cursor of its own.
	caret := len([]rune(written))
	if !look.Masked {
		caret = len([]rune(buffer.Text[:min(buffer.Caret, len(buffer.Text))]))
	}
	head, under, tail := splitAtCaret(written, caret)
	caretStyle := lipgloss.NewStyle().Background(theme.Accent).Foreground(theme.OnAccent)
	return style.Width(width).Render(
		present.TruncateText(head, width) + caretStyle.Render(under) + tail)
}

// readPasswordKey returns what one press does in the password prompt.
func (model *Model) readPasswordKey(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		model.screen = ScreenPickingProfile
		return model, nil
	case tea.KeyTab:
		if model.picker.offersKeyring() {
			model.picker.keepInKeyring = !model.picker.keepInKeyring
		}
		return model, nil
	case tea.KeyEnter:
		profile := model.picker.pending
		model.screen = ScreenConnecting
		return model, connect(model.adapters, profile, model.picker.password.Text)
	case tea.KeyBackspace:
		model.picker.password.DeleteBackward()
		return model, nil
	case tea.KeyDelete:
		model.picker.password.DeleteForward()
		return model, nil
	case tea.KeyLeft:
		model.picker.password.MoveCaret(-1, false)
		return model, nil
	case tea.KeyRight:
		model.picker.password.MoveCaret(1, false)
		return model, nil
	}
	if key.Text != "" && !key.Mod.Contains(uv.ModCtrl) && !key.Mod.Contains(uv.ModAlt) {
		model.picker.password.Insert(key.Text)
	}
	return model, nil
}

// pasteIntoPassword writes what the terminal pasted into the password field.
func (model *Model) pasteIntoPassword(written string) (tea.Model, tea.Cmd) {
	if model.confirm != nil {
		return model, nil
	}
	model.picker.password.Insert(flattenPaste(written))
	return model, nil
}

// flattenPaste turns the breaks of a paste into spaces, because a field is one line.
func flattenPaste(written string) string {
	written = strings.ReplaceAll(strings.ReplaceAll(written, "\r\n", " "), "\r", " ")
	return strings.ReplaceAll(written, "\n", " ")
}
