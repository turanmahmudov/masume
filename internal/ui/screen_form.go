package ui

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/present"
)

// TestStateKind says how far a test of the form got.
type TestStateKind string

// The four states a test can be in.
const (
	TestIdle    TestStateKind = "idle"
	TestRunning TestStateKind = "running"
	TestPassed  TestStateKind = "passed"
	TestFailed  TestStateKind = "failed"
)

// FormState is the connection form: the fields, which one holds the caret, and how the last
// test went.
type FormState struct {
	Fields []cfg.FormField
	// The profile the form edits, so the fields it does not show survive a save.
	Source cfg.Profile
	// True while an existing profile is being edited rather than a new one written.
	Editing bool
	// Which of the shown fields holds the caret.
	Cursor int
	// The text of the field under the caret.
	Draft *app.EditorBuffer
	// How the last test of the connection went.
	Test    TestStateKind
	Message string
}

// NewFormState opens the form on a profile, or on a blank one for a new connection.
func NewFormState(profile cfg.Profile, editing bool) *FormState {
	form := &FormState{
		Fields: cfg.BuildFormFields(profile, editing), Source: profile, Editing: editing,
		Test: TestIdle,
	}
	form.openField()
	return form
}

// Shown returns the fields the form draws now, which follow the engine and the auth mode.
func (form *FormState) Shown() []cfg.FormField {
	return cfg.FindShownFields(form.Fields)
}

// openField puts the text of the field under the caret into the draft.
func (form *FormState) openField() {
	shown := form.Shown()
	if len(shown) == 0 {
		form.Draft = app.NewEditorBuffer("", 0)
		return
	}
	if form.Cursor >= len(shown) {
		form.Cursor = len(shown) - 1
	}
	if form.Cursor < 0 {
		form.Cursor = 0
	}
	value := shown[form.Cursor].Value
	form.Draft = app.NewEditorBuffer(value, len(value))
}

// keepField writes the draft back into the field under the caret.
func (form *FormState) keepField() {
	shown := form.Shown()
	if form.Cursor < 0 || form.Cursor >= len(shown) {
		return
	}
	form.Fields = cfg.ApplyFieldChange(form.Fields, shown[form.Cursor].Key, form.Draft.Text)
}

// StepField moves the caret to another field, and keeps what was typed into this one.
func (form *FormState) StepField(step int) {
	form.keepField()
	shown := form.Shown()
	form.Cursor = wrap(form.Cursor+step, len(shown))
	form.openField()
}

// StepChoice steps a field that offers a list of values through them.
func (form *FormState) StepChoice(step int) bool {
	shown := form.Shown()
	if form.Cursor < 0 || form.Cursor >= len(shown) {
		return false
	}
	field := shown[form.Cursor]
	if len(field.Choices) == 0 {
		return false
	}

	at := 0
	for index, choice := range field.Choices {
		if choice == form.Draft.Text {
			at = index
			break
		}
	}
	wanted := field.Choices[wrap(at+step, len(field.Choices))]
	form.Fields = cfg.ApplyFieldChange(form.Fields, field.Key, wanted)
	form.openField()
	return true
}

// BuildProfile returns the profile the form now describes, and refuses a wrong value.
func (form *FormState) BuildProfile() (cfg.Profile, error) {
	form.keepField()
	return cfg.BuildProfileFromFields(form.Fields, form.Source, form.Editing)
}

// formTestedMsg returns one test of the form.
type formTestedMsg struct {
	Problem string
}

// testFormConnection opens the profile the form describes and closes it again, so a wrong
// value is found before it is saved.
func testFormConnection(adapters engines.Adapters, profile cfg.Profile, password string) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), connectTimeout)
		defer stop()

		session, err := adapters.Open(ctx, profile, password)
		if err != nil {
			return formTestedMsg{Problem: db.DescribeError(err)}
		}
		_ = session.Close()
		return formTestedMsg{}
	}
}

// readFormKey returns what one press does in the connection form.
func (model *Model) readFormKey(key tea.Key) (tea.Model, tea.Cmd) {
	form := model.form
	if form == nil {
		model.screen = ScreenPickingProfile
		return model, nil
	}

	// Escape belongs to no action. It closes the form.
	if key.Code == tea.KeyEscape {
		model.screen = ScreenPickingProfile
		return model, nil
	}

	match, matched := model.keymap.MatchFirst(key,
		FindDialogActions("confirm"), cfg.ScopeDialog)
	if matched {
		if held, command, ran := model.runFormAction(match); ran {
			return held, command
		}
	}

	switch key.Code {
	case tea.KeyTab, tea.KeyDown:
		form.StepField(1)
		return model, nil
	case tea.KeyUp:
		form.StepField(-1)
		return model, nil
	case tea.KeyEnter:
		form.StepField(1)
		return model, nil
	case tea.KeyLeft:
		if !form.StepChoice(-1) {
			form.Draft.MoveCaret(-1, false)
		}
		return model, nil
	case tea.KeyRight:
		if !form.StepChoice(1) {
			form.Draft.MoveCaret(1, false)
		}
		return model, nil
	case tea.KeyBackspace:
		form.Draft.DeleteBackward()
		form.keepField()
		return model, nil
	case tea.KeyDelete:
		form.Draft.DeleteForward()
		form.keepField()
		return model, nil
	}

	if key.Text != "" && !key.Mod.Contains(uv.ModCtrl) && !key.Mod.Contains(uv.ModAlt) {
		form.Draft.Insert(key.Text)
		form.keepField()
	}
	return model, nil
}

// pasteIntoForm writes what the terminal pasted into the field the caret is in. The card
// tells the reader to paste a URL into the host field, so the paste has to land.
func (model *Model) pasteIntoForm(written string) (tea.Model, tea.Cmd) {
	form := model.form
	if form == nil || model.confirm != nil {
		return model, nil
	}
	// A field is one line, so the breaks of a paste are dropped rather than written.
	written = strings.ReplaceAll(strings.ReplaceAll(written, "\r\n", " "), "\r", " ")
	written = strings.ReplaceAll(written, "\n", " ")
	form.Draft.Insert(written)
	form.keepField()
	return model, nil
}

// runFormAction runs one action of the connection form, whether a key or a press asked for
// it. It reports whether the action belonged to the form.
func (model *Model) runFormAction(match Match) (tea.Model, tea.Cmd, bool) {
	form := model.form
	if form == nil {
		return model, nil, false
	}
	switch match.Action {
	case ActionSaveForm:
		held, command := model.saveForm()
		return held, command, true
	case ActionClose:
		model.screen = ScreenPickingProfile
		return model, nil, true
	case ActionTestConnection:
		profile, err := form.BuildProfile()
		if err != nil {
			form.Test, form.Message = TestFailed, err.Error()
			return model, nil, true
		}
		password, passwordErr := cfg.ResolveProfilePassword(profile)
		if passwordErr != nil {
			form.Test, form.Message = TestFailed, passwordErr.Error()
			return model, nil, true
		}
		form.Test, form.Message = TestRunning, ""
		return model, testFormConnection(model.adapters, profile, password), true
	}
	return model, nil, false
}

// saveForm writes the profile into the config file, and reports what it could not write.
func (model *Model) saveForm() (tea.Model, tea.Cmd) {
	form := model.form
	profile, err := form.BuildProfile()
	if err != nil {
		form.Test, form.Message = TestFailed, err.Error()
		return model, nil
	}

	replacing := ""
	if form.Editing {
		replacing = form.Source.Name
	}
	if writeErr := cfg.SaveProfileToFile(
		profile, replacing, cfg.ResolveConfigPath()); writeErr != nil {
		form.Test, form.Message = TestFailed, writeErr.Error()
		return model, nil
	}
	profile.InConfigFile = true

	model.profiles = replaceProfile(model.profiles, replacing, profile)
	// The file now holds it, so the question asked before the client ends does not offer
	// it again.
	model.unsaved = dropProfile(model.unsaved, replacing)
	model.unsaved = dropProfile(model.unsaved, profile.Name)
	// A profile the list does not hold leaves the cursor on the first row.
	at, _ := findProfileIndex(model.profiles, profile.Name)
	model.picker.focus(at, len(model.profiles))
	model.screen = ScreenPickingProfile
	model.form = nil
	return model, nil
}

// replaceProfile writes the profile back into the list, and drops the name it replaced.
func replaceProfile(profiles []cfg.Profile, replacing string, profile cfg.Profile) []cfg.Profile {
	kept := make([]cfg.Profile, 0, len(profiles)+1)
	written := false
	for _, held := range profiles {
		if held.Name == replacing || held.Name == profile.Name {
			if !written {
				kept = append(kept, profile)
				written = true
			}
			continue
		}
		kept = append(kept, held)
	}
	if !written {
		kept = append(kept, profile)
	}
	sortProfiles(kept)
	return kept
}

// findProfileIndex returns where the profile of that name stands, and whether the list holds
// one.
func findProfileIndex(profiles []cfg.Profile, name string) (int, bool) {
	for at, held := range profiles {
		if held.Name == name {
			return at, true
		}
	}
	return 0, false
}

func sortProfiles(profiles []cfg.Profile) {
	slices.SortStableFunc(profiles, func(left, right cfg.Profile) int {
		return strings.Compare(left.Name, right.Name)
	})
}

// askDeleteProfile asks before a profile is removed from the config file.
func (model *Model) askDeleteProfile(profile cfg.Profile) (tea.Model, tea.Cmd) {
	model.confirm = &confirmState{
		Title:       " delete connection ",
		Destructive: true,
		Body: "Remove \"" + profile.Name + "\" from the config file?\n\n" +
			cfg.DescribeProfileTarget(profile),
		Answer: func(confirmed bool) tea.Cmd {
			if !confirmed {
				return nil
			}
			if err := cfg.RemoveProfileFromFile(
				profile.Name, cfg.ResolveConfigPath()); err != nil {
				model.picker.problem = err.Error()
				return nil
			}
			model.profiles = dropProfile(model.profiles, profile.Name)
			model.unsaved = dropProfile(model.unsaved, profile.Name)
			model.picker.focus(model.picker.cursor, len(model.profiles))
			return nil
		},
	}
	return model, nil
}

func dropProfile(profiles []cfg.Profile, name string) []cfg.Profile {
	kept := make([]cfg.Profile, 0, len(profiles))
	for _, held := range profiles {
		if held.Name != name {
			kept = append(kept, held)
		}
	}
	return kept
}

// confirmState is a question a screen without a connection asks, which holds its own answer.
type confirmState struct {
	Title string
	Body  string
	// The words the card puts on the two answers. Empty gives "yes" and "no".
	Yes string
	No  string
	// True for a question whose answer cannot be taken back, which the card draws in the
	// colour of an error.
	Destructive bool
	// Answer runs for both answers. Escape closes the question without either.
	Answer func(bool) tea.Cmd
}

// The widths of the connection form.
const (
	widestFormCard    = 76
	narrowestFormCard = 40
	formLabelWidth    = 18
)

// renderForm draws the connection form: one row per field, and how the last test went.
func (model *Model) renderForm() string {
	form := model.form
	cardWidth := present.ResolveCardWidth(widestFormCard, narrowestFormCard, model.width)
	valueWidth := max(cardWidth-6-formLabelWidth, 8)

	title := " new connection "
	if form.Editing {
		title = " edit " + form.Source.Name + " "
	}

	// The card stands under the title bar, and its height follows what it holds.
	cardRows := len(form.Shown()) + formCardChrome
	if hasFormField(form.Shown(), "host") {
		cardRows++
	}
	left := halfRoundedUp(model.width - cardWidth)
	cardTop := titleBarRows + halfRoundedUp(model.height-2-cardRows)
	model.layout.formChoices = nil

	lines := []string{}
	for index, field := range form.Shown() {
		focused := index == form.Cursor
		marker := "  "
		labelStyle := model.styles.Muted()
		if focused {
			marker = present.FitText(model.icons.Icon(cfg.IconField), fieldMarkerWidth)
			labelStyle = model.styles.Accent()
		}
		label := marker + present.FitText(field.Label, formLabelWidth)

		value := field.Value
		if focused {
			value = form.Draft.Text
		}
		// Only the field being typed into stands on a ground of its own, so a row of
		// fields does not read as one dark block.
		look := FieldLook{
			Ground: model.styles.Theme.Panel, Ink: model.styles.Theme.Muted,
			Masked: field.Key == "password", Placeholder: cfg.DescribeFormValue(field),
		}
		if focused {
			look.Ground, look.Ink, look.Focused =
				model.styles.Theme.Header, model.styles.Theme.Text, true
		}
		written := model.renderField(
			app.NewEditorBuffer(value, len(value)), valueWidth, look)
		if len(field.Choices) > 0 {
			written = model.renderChoiceField(value, valueWidth, index,
				cardTop+cardBodyRow+len(lines),
				left+cardBodyColumn+present.MeasureText(label), focused)
		}
		lines = append(lines, labelStyle.Render(label)+written)
	}

	lines = append(lines, "")
	switch form.Test {
	case TestRunning:
		lines = append(lines, model.styles.Accent().Render("testing…"))
	case TestPassed:
		lines = append(lines, paintText(model.styles.Theme.Success, nil, "ok · "+form.Message))
	case TestFailed:
		lines = append(lines, model.styles.Error().Render(
			present.TruncateText(form.Message, cardWidth-4)))
	default:
		lines = append(lines, model.styles.Muted().Render("not tested"))
	}

	keys := model.sayKeys().name("↑↓", "field").
		bind(cfg.ScopeDialog, ActionTestConnection, "test").
		bind(cfg.ScopeDialog, ActionSaveForm, "save").
		bind(cfg.ScopeDialog, ActionClose, "cancel")
	// The keys are cut rather than wrapped, because the card keeps one row for them.
	said := present.TruncateText(keys.buildText(), cardWidth-4)
	lines = append(lines, model.renderKeyLine(keys, []string{said},
		cardTop+cardBodyRow+len(lines), left+cardBodyColumn, model.styles.Theme.Panel)[0])
	if hasFormField(form.Shown(), "host") {
		lines = append(lines, model.styles.Faint().Render(present.TruncateText(
			"paste a postgres:// or mysql:// url into host to fill the form", cardWidth-4)))
	}

	// Where the rows of the fields land on the screen, so a press marks the row it looks
	// like.
	model.layout.formRows = rowsHit{
		top:   cardTop + cardBodyRow,
		count: len(form.Shown()),
		from:  left + 1, to: left + cardWidth - 2,
	}
	return model.renderCard(title, cardWidth, lines, plainCard)
}

// fieldMarkerWidth is the column the mark of the field being written into keeps: the mark and
// the blank after it. Every field keeps it, so the names of the fields line up.
const fieldMarkerWidth = 2

// formCardChrome is the rows the card keeps besides its fields: the blank row, the row that
// reports the test, the row of keys, the two borders and the blank row inside each one.
const formCardChrome = 7

// hasFormField is true where the form shows the field of this key.
func hasFormField(fields []cfg.FormField, key string) bool {
	for _, field := range fields {
		if field.Key == key {
			return true
		}
	}
	return false
}

// The marks a field that steps through a list of values draws on each side of its value.
const (

	// choiceMarkChrome is the two marks and the blank beside each one.
	choiceMarkChrome = 4
)

// renderChoiceField draws a form field that steps through a list of values, with a mark on
// each side that a press steps it by. Only the field that holds the cursor draws the marks,
// because the value of the others is not being changed.
func (model *Model) renderChoiceField(
	value string, width, field, row, left int, focused bool,
) string {
	if !focused {
		return model.styles.Muted().Render(present.TruncateText(value, width))
	}
	theme := model.styles.Theme
	room := max(width-choiceMarkChrome, 1)
	// The marks stand beside the value itself, so a short value does not push the mark
	// that steps it on to the far end of the row.
	written := present.TruncateText(value, room)
	model.layout.formChoices = append(model.layout.formChoices, choiceHit{
		field: field, row: row,
		back: left, on: left + present.MeasureText(written) + 3,
	})
	return paintText(theme.Accent, theme.Panel, model.icons.Icon(cfg.IconStepBack)+" ") +
		paintText(theme.Text, theme.Panel, written) +
		paintText(theme.Accent, theme.Panel, " "+model.icons.Icon(cfg.IconStepOn)) +
		paintOn(theme.Panel, strings.Repeat(" ", room-present.MeasureText(written)))
}

// readConfirmKey returns a question a screen without a connection asked.
func (model *Model) readConfirmKey(key tea.Key) (tea.Model, tea.Cmd) {
	held := model.confirm
	if key.Code == tea.KeyEscape {
		model.confirm = nil
		return model, nil
	}

	match, matched := model.keymap.MatchFirst(key,
		FindDialogActions("form"), cfg.ScopeDialog)
	if !matched {
		return model, nil
	}
	switch match.Action {
	case ActionAnswerYes, ActionChooseRow:
		model.confirm = nil
		return model, held.Answer(true)
	case ActionAnswerNo:
		model.confirm = nil
		return model, held.Answer(false)
	case ActionClose:
		model.confirm = nil
		return model, nil
	}
	return model, nil
}
