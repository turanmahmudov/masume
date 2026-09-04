package ui

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/filepicker"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/load"
	"github.com/turanmahmudov/masume/internal/present"
)

// The file picker of the import: the directory it opens in, the files it offers, and how it
// is drawn in the colours of the client.

// fileSizeWidth is the room the size of a file takes, which is enough for `1.1GB`.
const fileSizeWidth = 7

// pickerRows is how many rows of files the picker draws.
const pickerRows = 12

// buildFilePicker returns the picker of one import, opened in the directory the client was
// started in and offering the files an import can read.
func (model *Model) buildFilePicker() filepicker.Model {
	picker := filepicker.New()
	picker.AllowedTypes = load.ListFileExtensions()
	picker.DirAllowed = false
	picker.FileAllowed = true
	picker.AutoHeight = false
	picker.SetHeight(pickerRows)
	picker.ShowPermissions = false
	picker.ShowSize = true
	picker.Cursor = model.icons.Icon(cfg.IconField)
	if directory, err := os.Getwd(); err == nil {
		picker.CurrentDirectory = directory
	}
	picker.Styles = model.buildPickerStyles()
	return picker
}

// buildPickerStyles paints the picker in the theme of the client.
func (model *Model) buildPickerStyles() filepicker.Styles {
	theme := model.styles.Theme
	plain := lipgloss.NewStyle()
	return filepicker.Styles{
		Cursor:           plain.Foreground(theme.Accent),
		DisabledCursor:   plain.Foreground(theme.Muted),
		Symlink:          plain.Foreground(theme.Muted),
		Directory:        plain.Foreground(theme.Accent),
		File:             plain.Foreground(theme.Text),
		DisabledFile:     plain.Foreground(theme.Muted),
		Permission:       plain.Foreground(theme.Muted),
		Selected:         plain.Foreground(theme.OnAccent).Background(theme.Accent),
		DisabledSelected: plain.Foreground(theme.Muted),
		// The component writes the row under the cursor with the size to the right of
		// its room, so every other row is set to match it.
		FileSize: plain.Foreground(theme.Muted).
			Width(fileSizeWidth).Align(lipgloss.Right),
		EmptyDirectory: plain.Foreground(theme.Muted).SetString("no file this import can read"),
	}
}

// openFilePicker gives the import of this connection a picker, and returns the command that
// reads the directory it opens in.
func (model *Model) openFilePicker(connectionID int) tea.Cmd {
	if model.importPickers == nil {
		model.importPickers = map[int]*filepicker.Model{}
	}
	picker := model.buildFilePicker()
	model.importPickers[connectionID] = &picker
	return picker.Init()
}

// findFilePicker returns the picker of the import that is open on this connection, and
// nothing where no import is picking a file.
func (model *Model) findFilePicker(connectionID int) *filepicker.Model {
	return model.importPickers[connectionID]
}

// readPickerMessage hands a message to the picker of the import that is picking a file, which
// reads a directory with a command of its own.
func (model *Model) readPickerMessage(message tea.Msg) (tea.Model, tea.Cmd, bool) {
	connection, id := model.Active(), model.ActiveID()
	if connection == nil || connection.Overlay.Kind != app.OverlayImport ||
		connection.Overlay.Import.Stage != app.ImportPick {
		return model, nil, false
	}
	picker := model.findFilePicker(id)
	if picker == nil {
		return model, nil, false
	}

	held, command := picker.Update(message)
	*picker = held
	if chosen, path := held.DidSelectFile(message); chosen {
		return model.readPickedFile(connection, id, path)
	}
	return model, command, true
}

// readPickedFile takes the file the user picked into the form, and reads it at once.
func (model *Model) readPickedFile(
	connection *app.Connection, connectionID int, path string,
) (tea.Model, tea.Cmd, bool) {
	overlay := &connection.Overlay
	overlay.Import.Plan.Path = path
	overlay.Import.Stage = app.ImportFile
	overlay.Notice = ""
	ApplyImportPath(overlay)
	// The row is written to directly, because the step of a cursor writes what the row it
	// leaves held and this row was never typed into.
	overlay.Field = importFileField
	overlay.Draft = app.NewEditorBuffer(path, len(path))

	overlay.Import.Running = true
	return model, readImportFile(connectionID, connection.Session, overlay.Import.Plan), true
}

// renderFilePicker draws the picker of the import, or the reason there is none to draw.
func (model *Model) renderFilePicker(connectionID int, width int) []string {
	picker := model.findFilePicker(connectionID)
	if picker == nil {
		return []string{model.styles.Muted().Render("the picker is not open")}
	}

	lines := []string{
		model.styles.Muted().Render(present.TruncateText(picker.CurrentDirectory, width)),
		"",
	}
	for line := range strings.SplitSeq(picker.View(), "\n") {
		lines = append(lines, truncateStyled(line, width))
	}
	return lines
}
