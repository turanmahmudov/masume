package ui

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/filepicker"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/present"
)

// The import file picker uses the current directory and theme.

// fileSizeWidth is the room the size of a file takes, which is enough for `1.1GB`.
const fileSizeWidth = 7

// pickerRows is how many rows of files the picker draws.
const pickerRows = 12

// buildFilePicker returns a picker opened in the directory the client was started in,
// offering the files of those extensions.
func (model *Model) buildFilePicker(extensions []string) filepicker.Model {
	picker := filepicker.New()
	picker.AllowedTypes = extensions
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
		EmptyDirectory: plain.Foreground(theme.Muted).SetString("no file of that kind here"),
	}
}

// openFilePicker gives this connection a picker of those files, and returns the command that
// reads the directory it opens in.
func (model *Model) openFilePicker(connectionID int, extensions []string) tea.Cmd {
	if model.filePickers == nil {
		model.filePickers = map[int]*filepicker.Model{}
	}
	picker := model.buildFilePicker(extensions)
	model.filePickers[connectionID] = &picker
	return picker.Init()
}

// findFilePicker returns the picker open on this connection, and nothing where no card is
// picking a file.
func (model *Model) findFilePicker(connectionID int) *filepicker.Model {
	return model.filePickers[connectionID]
}

// picksFile is true while the card of this connection is picking a file.
func picksFile(overlay app.Overlay) bool {
	return (overlay.Kind == app.OverlayImport && overlay.Import.Stage == app.ImportPick) ||
		(overlay.Kind == app.OverlayDump && overlay.Dump.Stage == app.DumpPick)
}

// readPickerMessage hands a message to the picker of the card that is picking a file, which
// reads a directory with a command of its own.
func (model *Model) readPickerMessage(message tea.Msg) (tea.Model, tea.Cmd, bool) {
	connection, id := model.Active(), model.ActiveID()
	if connection == nil || !picksFile(connection.Overlay) {
		return model, nil, false
	}
	picker := model.findFilePicker(id)
	if picker == nil {
		return model, nil, false
	}

	held, command := picker.Update(message)
	*picker = held
	chosen, path := held.DidSelectFile(message)
	if !chosen {
		return model, command, true
	}
	if connection.Overlay.Kind == app.OverlayDump {
		model.readRestoreFile(connection, path)
		return model, nil, true
	}
	return model.readPickedFile(connection, id, path)
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
