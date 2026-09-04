package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/filepicker"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/present"
)

// ScreenKind names which screen is active. Each one carries its own data, so no screen can
// read state that does not belong to it.
type ScreenKind string

// The five screens the client draws.
const (
	ScreenPickingProfile    ScreenKind = "picking-profile"
	ScreenPromptingPassword ScreenKind = "prompting-password"
	ScreenConnecting        ScreenKind = "connecting"
	ScreenEditingConnection ScreenKind = "editing-connection"
	ScreenWorking           ScreenKind = "working"
)

// historyLimit is how many statements the history overlay lists.
const historyLimit = 100

// Model is the root of the client: which screen is active, the open connections, and
// everything the frame is drawn from.
type Model struct {
	width  int
	height int

	styles   *Styles
	icons    IconSet
	registry *KeyRegistry
	keymap   *Keymap
	adapters engines.Adapters
	log      *hist.Store
	settings cfg.UISettings
	// The chat settings the config file carried, and the provider the palette chose.
	ai         cfg.AiConfig
	aiProvider cfg.AiProviderID

	// The file picker of the import that is open on each connection. It is kept here
	// because it answers with commands of the draw loop.
	importPickers map[int]*filepicker.Model

	profiles []cfg.Profile
	// What the config and the theme files got wrong, which the palette lists.
	problems []string
	// The profile the command line named, which is opened as the client starts instead
	// of drawing the picker.
	startProfile *cfg.Profile
	// The connections that were opened and are in no config file, so the client offers to
	// write them to it before it ends.
	unsaved []cfg.Profile

	screen ScreenKind
	picker pickerState
	// The connection form, which is a screen of its own.
	form *FormState

	connections openConnections

	runs runBatches
	// A question a screen without a connection asks, which holds its own answer.
	confirm *confirmState
	// The frame of the wheel drawn while something runs.
	spinnerAt int
	quitting  bool

	terminal terminalColorState
	// Where the caret of the editor last drew, so the completion popup stands under it.
	caretRow    int
	caretColumn int
	editorLeft  int

	// Where the parts of the last frame were drawn, so a press of a button can be read
	// as a press on a row, a cell or a tab.
	layout frameLayout
	// The clicks in a row on one target, because a terminal reports no double click.
	clicks clickCounter
	// What the pointer was dragged over, which `Ctrl+C` copies.
	selection screenSelection
	drag      pointerDrag
	// What this client last put on the clipboard. The terminal owns the system clipboard
	// and returns no read of it, so a paste inside the client reads this.
	clipboard string
	frame     screenFrame
	// The keys the card on show names at its foot, kept while the frame is drawn so the
	// status bar under the card names them too and a press on one runs it.
	cardKeys *KeyLine
	caches   tabCaches
	// The conversation as the chat panel draws it, kept because the scroll bounds, a jump
	// between turns and the draw itself each read the rows.
	chatRows chatRowsCache
	// What the last copy took, for the screens without a connection to report it.
	copied string
}

// NewModel builds the root of the client from what the config file gave.
func NewModel(
	loaded cfg.LoadedConfig, adapters engines.Adapters, log *hist.Store, problems []string,
) *Model {
	registryOfThemes := NewThemeRegistry()
	styles := NewStyles(registryOfThemes)

	found := append([]string{}, problems...)
	found = append(found, registryOfThemes.ListBuiltInProblems()...)
	found = append(found, registryOfThemes.RegisterDocuments(loaded.Themes)...)
	found = append(found, loaded.Settings.ColorProblems...)
	found = append(found, loaded.Settings.Problems...)
	found = append(found, loaded.Ai.Problems...)
	found = append(found, styles.ApplyColorOverrides(loaded.Settings.Colors)...)
	if loaded.Settings.Theme != "" {
		reported, applied := styles.ApplyThemeByName(loaded.Settings.Theme)
		if !applied {
			found = append(found, "theme \""+loaded.Settings.Theme+"\" is not one there is")
		}
		found = append(found, reported...)
	}

	keys := NewKeyRegistry()
	for _, problem := range keys.ApplyKeySettings(
		FindKeyPreset(loaded.Keys.Preset), loaded.Keys.Choices, loaded.Ai.Enabled) {
		found = append(found, "keys: "+problem)
	}
	for _, problem := range loaded.Keys.Problems {
		found = append(found, "keys: "+problem)
	}
	return &Model{
		width: 80, height: 24,
		styles: styles, registry: keys, keymap: NewKeymap(keys),
		icons:    BuildIconSet(loaded.Settings.IconSet, loaded.Settings.IconGlyphs),
		adapters: adapters, log: log, settings: loaded.Settings,
		ai: loaded.Ai, aiProvider: loaded.Ai.DefaultProvider,
		profiles: loaded.Profiles, problems: found,
		screen: ScreenPickingProfile,
		picker: pickerState{password: app.NewEditorBuffer("", 0)},
		// A theme that follows the terminal has no colours until the terminal returns, so
		// the first frame waits for it.
		terminal: newTerminalColorState(styles.FollowsTerminal()),
	}
}

// OpenAtStart names the profile the client connects to as it opens.
func (model *Model) OpenAtStart(profile cfg.Profile) {
	model.startProfile = &profile
	for index, held := range model.profiles {
		if held.Name == profile.Name {
			model.picker.focus(index, len(model.profiles))
		}
	}
}

// Init asks the terminal for its own colours, so a theme that follows the terminal is drawn
// in them at once.
func (model *Model) Init() tea.Cmd {
	commands := []tea.Cmd{
		// A terminal that reports a change of its own theme is asked to do so, which
		// costs one sequence and returns the moment the user switches light and dark.
		tea.Raw(ansi.SetMode(ansi.ModeLightDark)),
		func() tea.Msg { return tea.RequestBackgroundColor() },
		func() tea.Msg { return tea.RequestForegroundColor() },
		RequestTerminalPalette(),
		waitForTerminalColors(),
		tick(spinnerFrameWait),
	}
	if model.startProfile != nil {
		if _, opening := model.chooseProfile(*model.startProfile); opening != nil {
			commands = append(commands, opening)
		}
	}
	return tea.Batch(commands...)
}

// dashboardRefreshWait is how long the dashboard leaves between two reads of the server. It
// rides the wake the client already asks for rather than a clock of its own.
const dashboardRefreshWait = 2 * time.Second

// refreshDashboard reads the server again where the dashboard is open and the last answer
// has gone stale. A read that is already on its way is left to land.
func (model *Model) refreshDashboard(now time.Time) tea.Cmd {
	connection := model.Active()
	if connection == nil || model.screen != ScreenWorking {
		return nil
	}
	overlay := &connection.Overlay
	if overlay.Kind != app.OverlayActivity || overlay.View.Reading {
		return nil
	}
	if now.Sub(overlay.Server.ReadAt) < dashboardRefreshWait {
		return nil
	}
	overlay.View.Reading = true
	return readActivity(model.ActiveID(), connection.Session, readRefresh)
}

// resolveTickWait returns how long until the next wake. A wheel that turns needs a frame ten
// times a second; a client with nothing to wait for needs only to take a report off the bar.
func (model *Model) resolveTickWait() time.Duration {
	if model.isWaitingForSomething() {
		return spinnerFrameWait
	}
	return restingWait
}

// isWaitingForSomething is true while any wheel on screen has something to turn for.
func (model *Model) isWaitingForSomething() bool {
	if model.screen == ScreenConnecting {
		return true
	}
	if model.form != nil && model.form.Test == TestRunning {
		return true
	}
	if model.runs.count() > 0 {
		return true
	}
	for _, connection := range model.connections.all() {
		if connection.Catalog.Loading || connection.Health == app.HealthReconnecting {
			return true
		}
		if connection.Chat != nil && connection.Chat.IsStreaming() {
			return true
		}
		for _, tab := range connection.Tabs {
			if tab.ViewData.Kind == app.DataLoading || tab.Results.IsRunning() ||
				tab.Results.IsCounting() {
				return true
			}
		}
	}
	return false
}

// Active returns the connection on screen, and nothing before the first one opens.
func (model *Model) Active() *app.Connection { return model.connections.active() }

// ActiveID returns the id of the connection on screen.
func (model *Model) ActiveID() int { return model.connections.activeID() }

// tabKey names one tab of one connection. A tab is numbered inside its own connection, so the
// number of the tab alone would let a tab of one connection read what was kept for the tab of
// another, and the grid would draw the rows of a server nobody asked about.
type tabKey struct {
	connection int
	tab        int
}

// buildTabKey names the tab of that connection, for everything kept per tab.
func (model *Model) buildTabKey(connection *app.Connection, tab *app.Tab) tabKey {
	return tabKey{connection: model.connections.idOf(connection), tab: tab.ID}
}

// forgetClosedTabs drops what was kept for a tab or a connection that is no longer open. One
// entry holds a whole page of rows as text, so a session that opens and closes many tabs would
// hold on to every page it ever drew.
//
// What goes is decided by which tabs are open, and not by a call at each place a tab closes:
// there are several such places, and one that forgot to call would leak again.
func (model *Model) forgetClosedTabs() {
	model.caches.forgetClosed(model.connections.holdsTab)
}

// findConnection returns the connection of that id, and its position.
func (model *Model) findConnection(id int) (*app.Connection, int, bool) {
	return model.connections.find(id)
}

// Update reads one message and returns the work it asks for.
func (model *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	// Only a move of the pointer that changes nothing holds the frame, and it says so
	// itself, so every other message draws again.
	model.frame.held = false

	switch held := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = held.Width, held.Height
		return model, nil

	case terminalColorsWaitedMsg:
		model.terminal.waiting = false
		return model, nil

	// The ground and the ink arrive on their own where the terminal reports a change of
	// its theme, and the slots of its palette change with them, so those are asked for
	// again whenever one of these is new.
	case tea.BackgroundColorMsg:
		if !model.terminal.keepGround(held.Color) {
			return model, nil
		}
		model.applyTerminalColors()
		return model, model.askTerminalPaletteAgain()

	case tea.ForegroundColorMsg:
		if !model.terminal.keepInk(held.Color) {
			return model, nil
		}
		model.applyTerminalColors()
		return model, model.askTerminalPaletteAgain()

	// A palette slot arrives in an answer no message of the engine covers, so the raw
	// sequence is read here.
	case uv.UnknownOscEvent:
		slot, reported, answered := readPaletteAnswer(held)
		if !answered || !model.terminal.keepSlot(slot, reported) {
			return model, nil
		}
		model.applyTerminalColors()
		return model, nil

	case tea.MouseClickMsg:
		return model.readMouse(held)

	case tea.MouseWheelMsg:
		return model.readMouseWheel(held)

	case tea.MouseMotionMsg:
		return model.readMouseMotion(held)

	case tea.MouseReleaseMsg:
		return model.readMouseRelease(held)

	// A wake carries no work: it is asked for so a mark that has run out is taken off the
	// frame at the moment it runs out, and not at the next turn of the wheel.
	case wakeMsg:
		return model, nil

	case tickMsg:
		model.spinnerAt++
		for _, connection := range model.connections.all() {
			connection.DropStaleNotice(time.Time(held))
		}
		return model, tea.Batch(
			tick(model.resolveTickWait()), model.askTerminalAgain(time.Time(held)),
			model.refreshDashboard(time.Time(held)))

	case formTestedMsg:
		if model.form != nil {
			if held.Problem == "" {
				model.form.Test, model.form.Message = TestPassed, ""
			} else {
				model.form.Test, model.form.Message = TestFailed, held.Problem
			}
		}
		return model, nil

	case transactionRanMsg:
		return model.readTransactionAnswer(held)

	case cancelledMsg:
		return model.readCancelAnswer(held)

	case stoppedBackendMsg:
		return model.readStopBackendAnswer(held)

	case tea.KeyPressMsg:
		return model.readKey(held.Key())

	// The terminal reports a paste of its own as one message, whatever key the user pressed
	// for it, so the whole of the text lands in the buffer in one edit.
	case tea.PasteMsg:
		return model.readPaste(held.Content)

	case connectedMsg:
		return model.readConnected(held)

	case catalogReadMsg:
		return model.readCatalogAnswer(held)

	case diagramMsg:
		return model.readDiagramAnswer(held)

	case importReadMsg:
		return model.readImportAnswer(held)

	case importCheckedMsg:
		return model.readImportCheck(held)

	case importRanMsg:
		return model.readImportRun(held)

	case healthDueMsg:
		return model.readHealthDue(held)

	case healthCheckedMsg:
		return model.readHealthChecked(held)

	case reconnectedMsg:
		return model.readReconnected(held)

	case checkDueMsg:
		return model.readCheckDue(held)

	case checkedMsg:
		return model.readChecked(held)

	case chatEventsMsg:
		return model.readChatEvents(held)

	case chatClosedMsg:
		return model.readChatClosed(held)

	case tableDetailMsg:
		return model.readTableDetailAnswer(held)

	case queryRanMsg:
		return model.readQueryAnswer(held)

	case pageReadMsg:
		return model.readPageAnswer(held)

	case countedMsg:
		return model.readCountAnswer(held)

	case planReadMsg:
		return model.readPlanAnswer(held)

	case relationViewMsg:
		return model.readRelationViewAnswer(held)

	case changesAppliedMsg:
		return model.readChangesAnswer(held)

	case historyReadMsg:
		return model.readHistoryAnswer(held)

	case savedReadMsg:
		return model.readSavedAnswer(held)

	case activityReadMsg:
		return model.readActivityAnswer(held)

	case marksReadMsg:
		return model.readMarksAnswer(held)

	case exportWrittenMsg:
		return model.readExportAnswer(held)

	case historyWrittenMsg:
		return model.readHistoryWritten(held)

	case savedQueryRemovedMsg:
		return model.readSavedQueryRemoved(held)

	case conversationKeptMsg:
		return model.readConversationKept(held)
	}

	if held, command, taken := model.readPickerMessage(message); taken {
		return held, command
	}
	return model, nil
}

// applyTerminalColors hands the theme what the terminal reported about itself.
func (model *Model) applyTerminalColors() {
	model.styles.ApplyTerminalColors(model.terminal.describe())
	if model.terminal.hasEvery() {
		model.terminal.waiting = false
	}
}

// askTerminalPaletteAgain asks for the sixteen slots again. The terminal reports a change of
// its ground or its ink on its own, and its palette changes with them.
func (model *Model) askTerminalPaletteAgain() tea.Cmd {
	if !model.styles.FollowsTerminal() {
		return nil
	}
	return RequestTerminalPalette()
}

// askTerminalAgain asks the terminal for its colours again while it has not answered for all
// of them, with the wait doubled after each ask. A window the compositor stopped drawing
// returns nothing, and it must not keep the standard palette once it is drawn again.
func (model *Model) askTerminalAgain(now time.Time) tea.Cmd {
	if !model.styles.FollowsTerminal() {
		return nil
	}
	// Every colour has arrived. A terminal reports a switch of its own theme, and reports
	// nothing at all when its palette is edited, so the colours are read again on a slow
	// beat. An answer that names the colour already held repaints nothing.
	if model.terminal.hasEvery() {
		if !model.terminal.takeWatch(now) {
			return nil
		}
		return requestTerminalColors()
	}
	if !model.terminal.takeAsk() {
		return nil
	}
	return requestTerminalColors()
}

func requestTerminalColors() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return tea.RequestBackgroundColor() },
		func() tea.Msg { return tea.RequestForegroundColor() },
		RequestTerminalPalette(),
	)
}

// View draws the frame: the title bar, the screen, and the status bar under it.
//
// The ground of the terminal itself is left alone. Every cell of the frame carries its own
// ground, and a client that painted the terminal would read its own colour back when it asks
// the terminal what ground it has.
func (model *Model) View() tea.View {
	if model.terminal.waiting {
		// Nothing is drawn until the colours are known, so a theme that follows the terminal
		// is never drawn first in the standard palette and then in the colours of the
		// terminal.
		view := tea.NewView("")
		view.AltScreen = true
		return view
	}
	// Only the pointer moved while the frame is held, so the frame it drew still stands and
	// only the marks laid over it can have changed.
	drawn := model.frame.needsDraw()
	if drawn {
		model.frame.text = model.render()
	}
	// The pointer is read against the frame that is on screen, so the mark stands on what
	// the reader sees rather than on what was there before.
	marks := viewMarks{
		hover:     model.resolveHover(model.frame.pointerX, model.frame.pointerY),
		selection: model.selection,
		pressed:   model.frame.pressed,
		lit:       model.frame.isFlashing(),
	}
	if drawn || model.frame.needsPaint(marks) {
		model.frame.keepMarks(marks)
		model.frame.shown = model.paintPressedKey(
			model.paintHover(model.paintSelection(model.frame.text)))
	}
	view := tea.NewView(model.frame.shown)
	view.AltScreen = true
	// Every move is reported, not only a drag, so the frame marks what the pointer stands
	// on. A move that changes nothing is answered with the frame already on screen.
	view.MouseMode = tea.MouseModeAllMotion
	view.WindowTitle = model.windowTitle()
	return view
}

// windowTitle names the connection on screen, so a terminal tab says which server it is on.
func (model *Model) windowTitle() string {
	connection := model.Active()
	if connection == nil {
		return "masume"
	}
	return "masume · " + connection.Profile().Name
}

// readTextToCopy returns what `Ctrl+C` would copy: the cells the pointer was dragged over,
// or the text the editor holds selected. It reports whether there is anything to copy.
func (model *Model) readTextToCopy() (string, bool) {
	if written := model.readSelectedText(model.frame.text); written != "" {
		return written, true
	}
	connection := model.Active()
	if model.screen != ScreenWorking || connection == nil {
		return "", false
	}
	tab := connection.Active()
	if tab == nil || !tab.Editor.HasSelection() {
		return "", false
	}
	return tab.Editor.Selection(), true
}

// holdsSelection is true while text stands selected, so `Ctrl+C` copies instead of quitting.
// The drag over the cells of the frame and the caret of the editor are the two places a
// selection lives, so it is read from them rather than kept beside them.
func (model *Model) holdsSelection() bool {
	if model.selection.held {
		return true
	}
	if model.screen != ScreenWorking {
		return false
	}
	connection := model.Active()
	if connection == nil {
		return false
	}
	tab := connection.Active()
	return tab != nil && tab.Editor.HasSelection()
}

// copySelection puts the text on the clipboard, lets the selection go, and says what it took.
func (model *Model) copySelection(written string) tea.Cmd {
	model.selection = screenSelection{}
	model.copied = describeCopied(written)
	if connection := model.Active(); connection != nil {
		if tab := connection.Active(); tab != nil {
			tab.Editor.ClearSelection()
		}
		connection.Show(model.copied)
	}
	return model.keepOnClipboard(written)
}

// keepOnClipboard puts the text on the clipboard of the terminal, and keeps a copy of it so
// a paste inside the client has something to write.
func (model *Model) keepOnClipboard(written string) tea.Cmd {
	model.clipboard = written
	return tea.SetClipboard(written)
}

// readKey returns what one press does on the screen that is active.
func (model *Model) readKey(key tea.Key) (tea.Model, tea.Cmd) {
	// Ctrl+C copies while something is selected, and quits when there is none.
	if key.Mod.Contains(uv.ModCtrl) && key.Code == 'c' {
		if written, copies := model.readTextToCopy(); copies {
			return model, model.copySelection(written)
		}
		// Quitting here would drop the connections the open question is asking about.
		if model.confirm != nil {
			return model, nil
		}
		if model.askSaveOnExit() {
			return model, nil
		}
		model.quitting = true
		return model, model.shutDown()
	}

	if model.confirm != nil {
		return model.readConfirmKey(key)
	}

	switch model.screen {
	case ScreenPickingProfile:
		return model.readPickerKey(key)
	case ScreenPromptingPassword:
		return model.readPasswordKey(key)
	case ScreenConnecting:
		// Escape belongs to no action. It closes what is open, here the wait for a server.
		if key.Code == tea.KeyEscape {
			model.screen = ScreenPickingProfile
			return model, nil
		}
		return model, nil
	case ScreenEditingConnection:
		return model.readFormKey(key)
	case ScreenWorking:
		return model.readWorkspaceKey(key)
	}
	return model, nil
}

// recordUnsavedConnection keeps a connection that is in no config file. One that was opened
// twice is kept one time.
func (model *Model) recordUnsavedConnection(profile cfg.Profile) {
	if profile.InConfigFile {
		return
	}
	if _, held := findProfileIndex(model.unsaved, profile.Name); held {
		return
	}
	model.unsaved = append(model.unsaved, profile)
}

// describeSaveOnExit returns the question the card asks.
func describeSaveOnExit(unsaved []cfg.Profile) string {
	asked := "Write \"" + unsaved[0].Name + "\" to the config file?"
	if len(unsaved) > 1 {
		asked = fmt.Sprintf("Write %d connections to the config file?", len(unsaved))
	}

	lines := []string{asked, ""}
	for _, profile := range unsaved {
		lines = append(lines, profile.Name+"  "+cfg.DescribeProfileTarget(profile))
	}
	if slices.ContainsFunc(unsaved, holdsWrittenPassword) {
		lines = append(lines, "", "The password is written to the file as well.")
	}
	return strings.Join(lines, "\n")
}

// holdsWrittenPassword is true for a connection whose password would go into the config file.
func holdsWrittenPassword(profile cfg.Profile) bool {
	return profile.Password != ""
}

// askSaveOnExit asks whether the connections that are in no config file are written to it
// before the client ends, and reports whether it asked.
func (model *Model) askSaveOnExit() bool {
	if len(model.unsaved) == 0 || model.confirm != nil {
		return false
	}
	unsaved := model.unsaved

	model.confirm = &confirmState{
		Title: " save connection ",
		Body:  describeSaveOnExit(unsaved),
		Yes:   "save and quit", No: "quit without saving",
		Answer: func(confirmed bool) tea.Cmd {
			if !confirmed {
				model.quitting = true
				return model.shutDown()
			}
			if problem := model.saveUnsavedConnections(unsaved); problem != "" {
				model.picker.problem = problem
				model.screen = ScreenPickingProfile
				return nil
			}
			model.quitting = true
			return model.shutDown()
		},
	}
	return true
}

// saveUnsavedConnections writes the connections to the config file and returns the reason
// the first one could not be written.
func (model *Model) saveUnsavedConnections(unsaved []cfg.Profile) string {
	written := 0
	for _, profile := range unsaved {
		if err := cfg.SaveProfileToFile(profile, "", cfg.ResolveConfigPath()); err != nil {
			return fmt.Sprintf("%s could not be written: %s%s",
				profile.Name, err.Error(), describeSavedSoFar(written))
		}
		profile.InConfigFile = true
		model.profiles = replaceProfile(model.profiles, profile.Name, profile)
		model.unsaved = dropProfile(model.unsaved, profile.Name)
		written++
	}
	return ""
}

// describeSavedSoFar returns how much of the file was written before a save stopped, and
// nothing where it stopped on the first one.
func describeSavedSoFar(written int) string {
	if written == 0 {
		return ""
	}
	return fmt.Sprintf(" (%s already written)", present.FormatCountOf(
		int64(written), "connection", "connections"))
}

// shutDown closes every connection and the history file, and ends the program. The tabs
// are written first, and in the sequence, so the last change a press made is in the file
// before the program ends.
func (model *Model) shutDown() tea.Cmd {
	commands := make([]tea.Cmd, 0, 2*model.connections.count()+1)
	for _, connection := range model.connections.all() {
		// A chat still asking the model holds the session the close below ends.
		connection.Chat.Stopped()
		commands = append(commands, model.saveWorkspace(connection))
	}
	for _, connection := range model.connections.all() {
		commands = append(commands, closeSession(connection.Session, connection.PreConnect))
	}
	commands = append(commands, tea.Quit)
	return tea.Sequence(commands...)
}

// refreshOpenConnection brings up the connection this profile already holds, and reads its
// object tree again. The picker names a profile, not a connection, so the list must not grow
// a second row for one that is open. It reports whether the profile was open.
func (model *Model) refreshOpenConnection(profile cfg.Profile) (tea.Cmd, bool) {
	for at, connection := range model.connections.all() {
		if connection.Profile().Name != profile.Name {
			continue
		}
		model.connections.focus(at)
		model.picker.problem = ""
		model.screen = ScreenWorking
		connection.Catalog.Loading = true
		return readCatalog(model.connections.idAt(at), connection.Session, announceCatalogRead), true
	}
	return nil, false
}

// chooseProfile opens the profile, and asks for a password first where only the user can
// give one.
func (model *Model) chooseProfile(profile cfg.Profile) (tea.Model, tea.Cmd) {
	if command, open := model.refreshOpenConnection(profile); open {
		return model, command
	}
	model.picker.problem = ""
	if cfg.NeedsPasswordPrompt(profile) {
		model.picker.askPassword(profile)
		model.screen = ScreenPromptingPassword
		return model, nil
	}

	password, err := cfg.ResolveProfilePassword(profile)
	if err != nil {
		model.picker.problem = err.Error()
		return model, nil
	}
	model.picker.pending = profile
	model.screen = ScreenConnecting
	return model, connect(model.adapters, profile, password)
}

// readConnected keeps the connection the server opened, or reports why it did not.
func (model *Model) readConnected(answered connectedMsg) (tea.Model, tea.Cmd) {
	// A user who left the screen is not thrown back onto it, and a profile that is no
	// longer the one being waited for is closed rather than opened: a slow server that
	// answers after the user went back and asked for another would otherwise take the
	// place of the one they asked for.
	if model.screen != ScreenConnecting || !model.picker.waitsFor(answered.Profile.Name) {
		if answered.Session != nil {
			return model, closeSession(answered.Session, answered.PreConnect)
		}
		return model, nil
	}

	if answered.Problem != "" {
		model.picker.problem = answered.Problem
		model.screen = ScreenPickingProfile
		return model, nil
	}

	connection := app.NewConnection(
		answered.Session, answered.PreConnect, model.settings.HideSystemSchemas)
	id := model.connections.open(connection)
	model.screen = ScreenWorking
	model.recordUnsavedConnection(connection.Profile())

	profileName := connection.Profile().Name
	// The profile opens with the tabs it was left with, or with one empty query tab.
	saved, held, savedErr := model.log.FindWorkspace(profileName)
	switch {
	case savedErr != nil:
		connection.ShowError("the tabs of this profile could not be read: " + savedErr.Error())
	case held:
		connection.RestoreTabs(saved, func(table db.TableRef) string {
			return connection.Session.Composer().ComposeRelationRead(
				table, core.ReadRewrite{}).Display
		})
	}

	commands := []tea.Cmd{
		readCatalog(id, connection.Session, quietCatalogRead),
		readMarks(id, model.log, profileName),
		scheduleHealthCheck(id, connection.Profile().Keepalive),
	}
	// A restored tab reads what it describes the first time it is shown.
	if _, command := model.readWhenShown(connection); command != nil {
		commands = append(commands, command)
	}
	return model, tea.Batch(commands...)
}

// readHealthDue asks the server whether it still returns.
func (model *Model) readHealthDue(due healthDueMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(due.ConnectionID)
	if !found {
		return model, nil
	}
	return model, pingSession(due.ConnectionID, connection.Session)
}

// readHealthChecked keeps what the check found, and arranges the next one. A server that
// stopped answering is reported once, so the bar and the dot say so without a key press.
func (model *Model) readHealthChecked(answered healthCheckedMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Problem == "" {
		if connection.Health != app.HealthOk {
			connection.Show("the server answers again")
		}
		connection.Health, connection.HealthFailures, connection.HealthProblem =
			app.HealthOk, 0, ""
		return model, scheduleHealthCheck(answered.ConnectionID, ResolveHealthBackoff(
			connection.HealthFailures, connection.Profile().Keepalive))
	}

	connection.HealthFailures++
	connection.HealthProblem = answered.Problem
	// A session that can be replaced is opened again at once. The bar says so while the
	// attempt runs, so a broken tunnel is visible without a key press.
	if reopen := reconnectSession(answered.ConnectionID, connection.Session); reopen != nil {
		connection.Health = app.HealthReconnecting
		return model, reopen
	}
	if connection.Health == app.HealthOk {
		connection.ShowError("the server did not answer: " + answered.Problem)
	}
	connection.Health = app.HealthDown
	return model, scheduleHealthCheck(answered.ConnectionID, ResolveHealthBackoff(
		connection.HealthFailures, connection.Profile().Keepalive))
}

// readReconnected keeps what an attempt at opening the connection again did. A connection
// that comes back does not bring the work back: the server rolled it back already.
func (model *Model) readReconnected(answered reconnectedMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Outcome.Reconnected {
		connection.Health, connection.HealthFailures, connection.HealthProblem =
			app.HealthOk, 0, ""
		if answered.Outcome.TransactionLost {
			connection.ShowError(
				"reconnected · the open transaction was lost with the connection")
		} else {
			connection.Show("reconnected")
		}
	} else {
		connection.Health = app.HealthDown
		problem := answered.Outcome.Problem
		if problem == "" {
			problem = "the server did not answer"
		}
		lost := ""
		if answered.Outcome.TransactionLost {
			lost = " · the open transaction went with the connection"
		}
		connection.ShowError("cannot reconnect: " + problem + lost)
	}
	return model, scheduleHealthCheck(answered.ConnectionID, ResolveHealthBackoff(
		connection.HealthFailures, connection.Profile().Keepalive))
}

// readCatalogAnswer keeps what the server holds.
func (model *Model) readCatalogAnswer(answered catalogReadMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}

	connection.Catalog.Loading = false
	connection.Catalog.ReadAt = time.Now()
	if answered.Problem != "" {
		connection.Catalog.Problem = answered.Problem
		connection.ShowError(answered.Problem)
		return model, nil
	}

	connection.Catalog.Problem = ""
	connection.Catalog.Tables = answered.Tables
	connection.Catalog.Objects = answered.Objects
	connection.Catalog.Roles = answered.Roles
	connection.Catalog.Details = map[string]present.TableDetailState{}
	connection.SeedTree()
	switch {
	case answered.PartProblem != "":
		connection.ShowError(answered.PartProblem)
	case answered.Announce:
		connection.Show("object tree reloaded")
	}

	// The columns of a relation the tree still holds open are read again, because the read
	// before them was dropped with the rest of the catalog.
	return model, tea.Batch(
		keepCatalog(answered.ConnectionID, model.log, connection.Profile().Name,
			answered.Tables, answered.Objects, answered.Roles),
		model.readOpenTableDetails(answered.ConnectionID, connection),
	)
}

// readOpenTableDetails asks the server again for the columns of every relation the tree holds
// open, so a row that was drawn falls back to nothing rather than to a read that never ends.
func (model *Model) readOpenTableDetails(id int, connection *app.Connection) tea.Cmd {
	commands := []tea.Cmd{}
	for _, row := range connection.BuildTree(time.Now()).Rows {
		if row.Node.Kind != present.NodeTable || !connection.Tree.Expanded[row.ID] {
			continue
		}
		tableID := present.BuildTableID(row.Node.Table)
		if _, reading := connection.Catalog.Details[tableID]; reading {
			continue
		}
		connection.Catalog.Details[tableID] = present.TableDetailState{
			Kind: present.DetailLoading,
		}
		commands = append(commands, readTableDetail(id, connection.Session, row.Node.Table))
	}
	if len(commands) == 0 {
		return nil
	}
	return tea.Batch(commands...)
}

// readTableDetailAnswer keeps the columns and the keys of one relation.
func (model *Model) readTableDetailAnswer(answered tableDetailMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Problem != "" {
		connection.Catalog.Details[answered.TableID] = present.TableDetailState{
			Kind: present.DetailFailed, Message: answered.Problem,
		}
		return model, nil
	}
	connection.Catalog.Details[answered.TableID] = present.TableDetailState{
		Kind: present.DetailReady, Detail: answered.Detail,
	}
	// A tab that waited for these columns can be written through now.
	model.resolveWaitingTargets(connection, answered.TableID)
	// The list under the caret is built again, so the columns that just arrived are in it.
	if tab := connection.Active(); tab != nil && tab.Focus == app.PaneEditor {
		model.refreshCompletion(connection, tab)
	}
	return model, nil
}

// resolveWaitingTargets builds the edit target again for every tab that reads this relation,
// because a target returns what it returns from the columns of it.
func (model *Model) resolveWaitingTargets(connection *app.Connection, tableID string) {
	for _, tab := range connection.Tabs {
		if tab.Target.Table.Name == "" ||
			present.BuildTableID(tab.Target.Table) != tableID {
			continue
		}
		tab.Target = model.resolveEditTarget(connection, tab)
	}
}

// readMarksAnswer keeps what the user marked on this profile.
func (model *Model) readMarksAnswer(answered marksReadMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	connection.Marks.Favourites = answered.Favourites
	connection.Marks.Recent = answered.Recent
	connection.SeedTree()
	return model, nil
}

// readHistoryAnswer draws the statements that ran on this profile.
func (model *Model) readHistoryAnswer(answered historyReadMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayHistory, Entries: answered.Entries,
		Draft: app.NewEditorBuffer("", 0),
	}
	return model, nil
}

// readSavedAnswer draws the statements a reader kept by name.
func (model *Model) readSavedAnswer(answered savedReadMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlaySaved, Saved: answered.Queries,
		Draft: app.NewEditorBuffer("", 0),
	}
	return model, nil
}

// readActivityAnswer draws what the server is doing. A refresh replaces what the read
// answered and keeps what the reader did to the card.
func (model *Model) readActivityAnswer(answered activityReadMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	held := connection.Overlay
	open := held.Kind == app.OverlayActivity
	if answered.Problem != "" {
		if open {
			held.View.Reading = false
			connection.Overlay = held
		}
		if !answered.Refresh {
			connection.ShowError(answered.Problem)
		}
		return model, nil
	}
	// A refresh whose card was closed would otherwise open it again under the reader.
	if answered.Refresh && !open {
		return model, nil
	}

	drawn := app.Overlay{
		Kind: app.OverlayActivity, Sessions: answered.Sessions,
		Server: app.ServerReading{
			Load: answered.Load, Locks: answered.Locks, Slow: answered.Slow,
			HasLoad: answered.HasLoad, HasLocks: answered.HasLocks,
			HasSlow: answered.HasSlow,
			ReadAt:  time.Now(),
		},
	}
	if open {
		drawn.List = held.List
		drawn.View = held.View
		// The reading being replaced is what the next rate is measured against, and is
		// kept only where both readings carry the counters.
		if held.Server.HasLoad && drawn.Server.HasLoad {
			drawn.View.Previous = held.Server.Load
			drawn.View.PreviousAt = held.Server.ReadAt
			drawn.View.HasPrevious = true
		}
	}
	drawn.View.Reading = false
	drawn.List.Cursor = clamp(drawn.List.Cursor, len(drawn.Sessions))
	connection.Overlay = drawn
	return model, nil
}

// readExportAnswer reports where the export was written.
func (model *Model) readExportAnswer(answered exportWrittenMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Problem != "" {
		connection.ShowError(answered.Problem)
		return model, nil
	}
	connection.Show(strings.Join([]string{
		"wrote", present.FormatRowCount(answered.Rows), "to", answered.Path,
	}, " "))
	return model, nil
}
