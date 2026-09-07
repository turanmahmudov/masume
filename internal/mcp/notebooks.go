package mcp

import (
	"context"
	"slices"

	"github.com/turanmahmudov/masume/internal/agent"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The notebook tools read files and send no statement. An agent that wants to run a cell
// sends its text through run_query, where the access level and the confirmation apply.

// buildListNotebooksTool returns the tool that lists the notebooks of the project and of
// the user.
func buildListNotebooksTool(deps ToolDeps) Tool {
	return Tool{
		Name: "list_notebooks",
		Description: "List the notebooks of the project and of the user: the `notebook` " +
			"name, its title, how many cells it holds, and how many of them write. " +
			"Read-only. Use read_notebook for the cells of one notebook.",
		InputSchema: agent.BuildEmptySchema(),
		Call: func(_ context.Context, _ map[string]any) (any, error) {
			open := ListOpenProfiles(deps.AccessDeps)
			if len(open) == 0 {
				return map[string]any{
					"notebooks": []any{}, "note": DescribeNoOpenProfiles(deps.Config),
				}, nil
			}
			described := []map[string]any{}
			for _, entry := range notebook.List(deps.ProjectFile, deps.NotebookPaths) {
				book, err := notebook.Read(entry.Path)
				if err != nil || !offersNotebook(book, open) {
					continue
				}
				described = append(described, map[string]any{
					"notebook": entry.Name, "title": book.Title,
					"origin": string(entry.Origin), "cells": len(book.Cells),
					"writes": book.CountWriteCells(),
					"engine": book.Engine, "profiles": listNames(book.Profiles),
				})
			}
			return map[string]any{"notebooks": described}, nil
		},
	}
}

// buildReadNotebookTool returns the tool that reads the cells of one notebook.
func buildReadNotebookTool(deps ToolDeps) Tool {
	return Tool{
		Name: "read_notebook",
		Description: "Read one notebook: its title, its run policy, and every cell with " +
			"its id, kind, title and text. Read-only; it runs no cell. Send the text of " +
			"a cell through run_query to run it.",
		InputSchema: agent.ExtendSchema(agent.BuildEmptySchema(), "notebook",
			"The notebook name from list_notebooks."),
		Call: func(_ context.Context, input map[string]any) (any, error) {
			name, isText := input["notebook"].(string)
			if !isText || name == "" {
				return nil, refuse(
					"notebook is required; call list_notebooks for available notebooks")
			}
			open := ListOpenProfiles(deps.AccessDeps)
			if len(open) == 0 {
				return nil, refuse("%s", DescribeNoOpenProfiles(deps.Config))
			}

			for _, entry := range notebook.List(deps.ProjectFile, deps.NotebookPaths) {
				if entry.Name != name {
					continue
				}
				book, err := notebook.Read(entry.Path)
				if err != nil {
					return nil, refuse("the notebook cannot be read: %s", err.Error())
				}
				if !offersNotebook(book, open) {
					return nil, refuse(
						"notebook %s is offered on no profile this server serves", name)
				}
				return describeNotebookForAgent(entry, book), nil
			}
			return nil, refuse(
				"notebook %s was not found; call list_notebooks for available notebooks",
				name)
		},
	}
}

// describeNotebookForAgent returns one notebook in the form the agent reads.
func describeNotebookForAgent(
	entry notebook.Entry, book notebook.Notebook,
) map[string]any {
	cells := make([]map[string]any, 0, len(book.Cells))
	for at, cell := range book.Cells {
		described := map[string]any{
			"cell": cell.ID, "position": at + 1, "kind": string(cell.Kind),
			"title": statement.FindQueryName(cell.Source), "text": cell.Source,
		}
		if cell.AsksConfirmation() {
			described["writes"] = true
		}
		cells = append(cells, described)
	}
	return map[string]any{
		"notebook": entry.Name, "title": book.Title, "origin": string(entry.Origin),
		"engine": book.Engine, "profiles": listNames(book.Profiles),
		"run": map[string]any{
			"transaction": book.Run.Transaction, "on_error": book.Run.OnError,
		},
		"cells": cells,
	}
}

// listNames returns the names as a list a JSON reader can walk, empty rather than absent.
func listNames(names []string) []string {
	if names == nil {
		return []string{}
	}
	return names
}

// offersNotebook is true where the front matter names a profile this server serves. A
// notebook that names none is offered on every profile.
func offersNotebook(book notebook.Notebook, open []cfg.Profile) bool {
	if len(book.Profiles) == 0 {
		return true
	}
	for _, profile := range open {
		if slices.Contains(book.Profiles, profile.Name) {
			return true
		}
	}
	return false
}
