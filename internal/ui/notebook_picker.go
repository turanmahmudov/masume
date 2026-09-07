package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
)

// The notebooks card lists the notebooks of the project and of the user. Opening one reads
// the file; it runs no cell.

// notebooksReadMsg carries the notebooks a read of the directories found.
type notebooksReadMsg struct {
	ConnectionID int
	Entries      []notebook.Entry
}

// notebookReadMsg carries one notebook file.
type notebookReadMsg struct {
	ConnectionID int
	Path         string
	Text         string
	Problem      string
	// True while the notebook opens in a tab of its own.
	InNewTab bool
}

// notebookWrittenMsg reports the write of one notebook file.
type notebookWrittenMsg struct {
	ConnectionID int
	TabID        int
	Path         string
	Problem      string
}

// notebookRemovedMsg reports the removal of one notebook file.
type notebookRemovedMsg struct {
	ConnectionID int
	Name         string
	Problem      string
}

// listNotebooks reads the notebook directories away from the loop.
func listNotebooks(connectionID int, projectFile string, extra []string) tea.Cmd {
	return func() tea.Msg {
		return notebooksReadMsg{
			ConnectionID: connectionID,
			Entries:      notebook.List(projectFile, extra),
		}
	}
}

// readNotebookFile reads one notebook file away from the loop.
func readNotebookFile(connectionID int, path string, inNewTab bool) tea.Cmd {
	return func() tea.Msg {
		answered := notebookReadMsg{
			ConnectionID: connectionID, Path: path, InNewTab: inNewTab,
		}
		text, err := os.ReadFile(core.ExpandHomePath(path))
		if err != nil {
			answered.Problem = "cannot read the notebook: " + db.DescribeError(err)
			return answered
		}
		answered.Text = string(text)
		return answered
	}
}

// writeNotebookFile writes one notebook file away from the loop.
func writeNotebookFile(
	connectionID, tabID int, path string, book notebook.Notebook,
) tea.Cmd {
	return func() tea.Msg {
		answered := notebookWrittenMsg{
			ConnectionID: connectionID, TabID: tabID, Path: path,
		}
		if err := notebook.Save(path, book); err != nil {
			answered.Problem = "cannot write the notebook: " + db.DescribeError(err)
		}
		return answered
	}
}

// removeNotebookFile deletes one notebook file away from the loop.
func removeNotebookFile(connectionID int, name, path string) tea.Cmd {
	return func() tea.Msg {
		answered := notebookRemovedMsg{ConnectionID: connectionID, Name: name}
		if err := os.Remove(core.ExpandHomePath(path)); err != nil {
			answered.Problem = "cannot delete the notebook: " + db.DescribeError(err)
		}
		return answered
	}
}

// openNewNotebook opens an empty notebook in a tab of its own.
func (model *Model) openNewNotebook(connection *app.Connection) (tea.Model, tea.Cmd) {
	tab := connection.OpenNotebook(app.NewNotebook(), "", "")
	tab.Focus = app.PaneEditor
	connection.Show("new notebook, not saved yet")
	return model, nil
}

// showNotebooks opens the card that lists the notebooks.
func (model *Model) showNotebooks(connection *app.Connection) (tea.Model, tea.Cmd) {
	connection.Overlay = app.Overlay{
		Kind: app.OverlayNotebooks, Draft: app.NewEditorBuffer("", 0),
	}
	return model, listNotebooks(
		model.ActiveID(), model.project.Path, model.notebooks.Paths)
}

// filterNotebooks returns the rows the term of the card keeps. A term that names a place
// keeps the notebooks kept there, so `project` lists the notebooks of the team.
func (model *Model) filterNotebooks(overlay app.Overlay) []notebook.Entry {
	term := strings.ToLower(model.readOverlayTerm(overlay))
	if term == "" {
		return overlay.Notebooks
	}
	kept := make([]notebook.Entry, 0, len(overlay.Notebooks))
	for _, entry := range overlay.Notebooks {
		held := strings.ToLower(entry.Name + " " + entry.Title + " " + string(entry.Origin))
		if strings.Contains(held, term) {
			kept = append(kept, entry)
		}
	}
	return kept
}

// renameNotebookRow asks for the new name of the notebook the cursor stands on.
func (model *Model) renameNotebookRow(
	connection *app.Connection, overlay *app.Overlay,
) (tea.Model, tea.Cmd) {
	entries := model.filterNotebooks(*overlay)
	if overlay.List.Cursor >= len(entries) {
		return model, nil
	}
	entry := entries[overlay.List.Cursor]
	connection.Overlay = app.Overlay{
		Kind: app.OverlayPrompt, Prompt: app.PromptNotebookRename,
		Title: "rename " + entry.Name,
		Hint:  "the file is renamed in " + core.ShortenHomePath(filepath.Dir(entry.Path)),
		// The file the rename acts on. No card draws this line.
		Body:  entry.Path,
		Draft: app.NewEditorBuffer(entry.Name, len(entry.Name)),
	}
	return model, nil
}

// answerNotebookRename renames the file the prompt was opened for.
func (model *Model) answerNotebookRename(
	connection *app.Connection, overlay app.Overlay, written string,
) (tea.Model, tea.Cmd) {
	if written == "" || overlay.Body == "" {
		return model, nil
	}
	held := notebook.ResolvePath(filepath.Dir(overlay.Body), written)
	if held == overlay.Body {
		return model, nil
	}
	return model, renameNotebookFile(
		model.ActiveID(), overlay.Body, held, notebook.ReadName(held))
}

// renameNotebookFile renames one notebook file away from the loop.
func renameNotebookFile(connectionID int, from, to, name string) tea.Cmd {
	return func() tea.Msg {
		answered := notebookRenamedMsg{
			ConnectionID: connectionID, Name: name, From: from, Path: to,
		}
		if _, err := os.Stat(to); err == nil {
			answered.Problem = "a notebook is already named " + name
			return answered
		}
		if err := os.Rename(from, to); err != nil {
			answered.Problem = "cannot rename the notebook: " + db.DescribeError(err)
		}
		return answered
	}
}

// notebookRenamedMsg reports the rename of one notebook file.
type notebookRenamedMsg struct {
	ConnectionID int
	Name         string
	// The file as it was, and as it is now.
	From    string
	Path    string
	Problem string
}

// readNotebookRenamed reports the rename and lists the notebooks again.
func (model *Model) readNotebookRenamed(answered notebookRenamedMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Problem != "" {
		connection.ShowError(answered.Problem)
		return model, nil
	}
	// A tab that holds the file follows the rename, so a save writes to the new name.
	for _, tab := range connection.Tabs {
		if tab.Kind != app.TabNotebook || tab.Notebook == nil {
			continue
		}
		if tab.Notebook.Path == answered.From {
			tab.Notebook.Path = answered.Path
		}
	}
	connection.Show("renamed to " + answered.Name)
	return model, listNotebooks(
		answered.ConnectionID, model.project.Path, model.notebooks.Paths)
}

// The widths of one row of the notebooks card.
const (
	notebookNameWidth = 24
	notebookLeadWidth = 9
)

// renderNotebooks draws the notebooks of the project and of the user.
func (model *Model) renderNotebooks(overlay app.Overlay, width int) string {
	entries := model.filterNotebooks(overlay)
	rows := make([]string, 0, len(entries))
	for at, entry := range entries {
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: string(entry.Origin), LeadWidth: notebookLeadWidth,
			Label: entry.Name, LabelWidth: notebookNameWidth,
			Detail:   describeNotebookEntry(entry),
			Selected: at == overlay.List.Cursor, Width: width,
		}))
	}
	keys := model.sayKeys().
		bind(cfg.ScopeList, ActionChooseRow, "open").
		bind(cfg.ScopeDialog, ActionOpenInNewTab, "open in a new tab").
		bind(cfg.ScopeDialog, ActionNewConnection, "new").
		bind(cfg.ScopeDialog, ActionEditConnection, "rename").
		bind(cfg.ScopeDialog, ActionDeleteConnection, "delete").
		bind(cfg.ScopeDialog, ActionClose, "close")
	return model.renderListCard(ListCard{
		Kind:   app.OverlayNotebooks,
		Title:  " notebooks · " + present.FormatCount(int64(len(overlay.Notebooks))) + " ",
		Filter: model.renderFilterFieldOf(overlay, width, "name", -1), Rows: rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset,
		Rolled: overlay.List.Rolled, Width: width,
		ReportsNoMatch: true, Keys: keys, ContentRows: len(overlay.Notebooks) + 1,
	})
}

// describeNotebookEntry returns what one row of the card says about a notebook: what it
// holds, when it changed, and the directory it is kept in.
func describeNotebookEntry(entry notebook.Entry) string {
	parts := []string{
		present.FormatCountOf(int64(entry.Cells), "cell", "cells"),
	}
	if !entry.ChangedAt.IsZero() {
		parts = append(parts, present.FormatWhen(entry.ChangedAt, time.Now()))
	}
	if entry.Writes > 0 {
		parts = append(parts, present.FormatCountOf(int64(entry.Writes), "write", "writes"))
	}
	if entry.Title != "" && entry.Title != entry.Name {
		parts = append(parts, entry.Title)
	}
	// The place of a project or a personal notebook is the label of its row, so only a
	// notebook of an extra directory says which directory it is.
	if entry.Origin == notebook.OriginExtra {
		parts = append(parts, core.ShortenHomePath(filepath.Dir(entry.Path)))
	}
	return strings.Join(parts, " · ")
}

// openNotebookRow opens the notebook the cursor of the card stands on.
func (model *Model) openNotebookRow(
	connection *app.Connection, overlay *app.Overlay, inNewTab bool,
) (tea.Model, tea.Cmd) {
	entries := model.filterNotebooks(*overlay)
	if overlay.List.Cursor >= len(entries) {
		return model, nil
	}
	entry := entries[overlay.List.Cursor]
	connection.Overlay = app.Overlay{}
	return model, readNotebookFile(model.ActiveID(), entry.Path, inNewTab)
}

// deleteNotebookRow deletes the notebook the cursor of the card stands on.
func (model *Model) deleteNotebookRow(
	connection *app.Connection, overlay *app.Overlay,
) (tea.Model, tea.Cmd) {
	entries := model.filterNotebooks(*overlay)
	if overlay.List.Cursor >= len(entries) {
		return model, nil
	}
	entry := entries[overlay.List.Cursor]
	held := *overlay
	connection.Overlay = app.Overlay{
		Kind: app.OverlayConfirm, Title: " delete this notebook ",
		Body: "The file " + core.ShortenHomePath(entry.Path) + " is deleted.",
		Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
			if !confirmed {
				connection.Overlay = held
				return nil
			}
			return carryAnswer(removeNotebookFile(
				model.ActiveID(), entry.Name, entry.Path))
		}},
	}
	return model, nil
}

// readNotebooksAnswer keeps the notebooks a read of the directories found.
func (model *Model) readNotebooksAnswer(answered notebooksReadMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found || connection.Overlay.Kind != app.OverlayNotebooks {
		return model, nil
	}
	connection.Overlay.Notebooks = answered.Entries
	connection.Overlay.List.Cursor = 0
	connection.Overlay.ContentRows = len(answered.Entries) + 1
	return model, nil
}

// readNotebookAnswer opens the notebook a read of a file returned.
func (model *Model) readNotebookAnswer(answered notebookReadMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Problem != "" {
		connection.ShowError(answered.Problem)
		return model, nil
	}
	book := notebook.Parse(answered.Text)
	if len(book.Problems) > 0 {
		connection.ShowError(book.Problems[0])
	}
	origin := model.resolveNotebookOrigin(answered.Path)
	open := connection.OpenNotebook
	if answered.InNewTab {
		open = connection.OpenNotebookInNewTab
	}
	tab := open(book, answered.Path, origin)
	tab.Focus = app.PaneEditor
	if book.CountWriteCells() > 0 {
		connection.Show(present.FormatCountOf(int64(book.CountWriteCells()),
			"write cell", "write cells") + "; nothing ran yet")
	}
	return model, model.saveWorkspace(connection)
}

// resolveNotebookOrigin returns where a path is kept.
func (model *Model) resolveNotebookOrigin(path string) notebook.Origin {
	directory := filepath.Dir(path)
	if project := notebook.ResolveProjectDirectory(model.project.Path); project != "" &&
		directory == project {
		return notebook.OriginProject
	}
	if directory == notebook.ResolvePersonalDirectory() {
		return notebook.OriginPersonal
	}
	return notebook.OriginExtra
}

// readNotebookWritten reports the write of one notebook file.
func (model *Model) readNotebookWritten(answered notebookWrittenMsg) (tea.Model, tea.Cmd) {
	connection, tab, found := model.findConnectionTab(answered.ConnectionID, answered.TabID)
	if !found {
		return model, nil
	}
	if answered.Problem != "" {
		connection.ShowError(answered.Problem)
		return model, nil
	}
	if tab.Notebook != nil {
		tab.Notebook.Path = answered.Path
		tab.Notebook.Origin = model.resolveNotebookOrigin(answered.Path)
		tab.Notebook.Dirty = false
	}
	connection.Show("written to " + core.ShortenHomePath(answered.Path))
	return model, model.saveWorkspace(connection)
}

// readNotebookRemoved reports the removal of one notebook file.
func (model *Model) readNotebookRemoved(answered notebookRemovedMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Problem != "" {
		connection.ShowError(answered.Problem)
		return model, nil
	}
	connection.Show(answered.Name + " deleted")
	return model, listNotebooks(
		answered.ConnectionID, model.project.Path, model.notebooks.Paths)
}

// saveNotebook writes the notebook of a tab. One that was never saved is asked for a name.
func (model *Model) saveNotebook(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	book := tab.Notebook
	if book == nil {
		return model, nil
	}
	if book.Path != "" {
		return model, writeNotebookFile(
			model.ActiveID(), tab.ID, book.Path, book.BuildDocument())
	}
	held := book.Title
	connection.Overlay = app.Overlay{
		Kind: app.OverlayPrompt, Prompt: app.PromptNotebookName, Title: "save as",
		Hint: "a plain name goes to " +
			core.ShortenHomePath(model.resolveNotebookDirectory()),
		Draft: app.NewEditorBuffer(held, len(held)),
	}
	return model, nil
}

// resolveNotebookDirectory returns the directory a notebook of a plain name is written to.
func (model *Model) resolveNotebookDirectory() string {
	if project := notebook.ResolveProjectDirectory(model.project.Path); project != "" {
		return project
	}
	return notebook.ResolvePersonalDirectory()
}

// answerNotebookName writes the notebook under the name the prompt took.
func (model *Model) answerNotebookName(
	connection *app.Connection, tab *app.Tab, written string,
) (tea.Model, tea.Cmd) {
	book := tab.Notebook
	if book == nil || written == "" {
		return model, nil
	}
	path := model.resolveWrittenPath(written)
	if book.Title == "" {
		// The name the reader typed is the title. The file name is that name as a slug,
		// and a title of a slug reads as one.
		book.Title = written
	}
	return model, writeNotebookFile(
		model.ActiveID(), tab.ID, path, book.BuildDocument())
}

// resolveWrittenPath reads the text of the prompt as a name or as a path.
func (model *Model) resolveWrittenPath(written string) string {
	expanded := core.ExpandHomePath(written)
	if strings.ContainsRune(expanded, filepath.Separator) {
		if !strings.HasSuffix(expanded, notebook.FileSuffix) {
			expanded += notebook.FileSuffix
		}
		return expanded
	}
	return notebook.ResolvePath(model.resolveNotebookDirectory(), written)
}
