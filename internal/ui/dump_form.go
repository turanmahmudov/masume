package ui

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/dump"
)

// The dump form contains the file and what the dump holds. A restore asks for the file
// alone.

// dumpPathField is the row of the form the file path is written on.
const dumpPathField = 0

// BuildDumpFields returns the rows of the form as the overlay stands now.
func BuildDumpFields(overlay app.Overlay) []DialogField {
	held := overlay.Dump
	fields := []DialogField{
		{Key: "path", Label: "file", Value: readDumpPath(overlay)},
	}
	if held.Mode == app.DumpRestore {
		return fields
	}

	contents := make([]string, 0, len(dump.Contents))
	for _, content := range dump.Contents {
		contents = append(contents, string(content))
	}
	return append(fields,
		DialogField{
			Key: "content", Label: "content",
			Value: string(held.Options.Content), Choices: contents,
		},
		DialogField{
			Key: "drop", Label: "drop first",
			Value: describeYesOrNo(held.Options.DropsFirst), Choices: yesOrNo,
		},
	)
}

// readDumpPath returns the path the form holds. The buffer follows the cursor, so it holds
// the path only while the cursor stands on that row.
func readDumpPath(overlay app.Overlay) string {
	if overlay.Draft != nil && overlay.Field == dumpPathField {
		return overlay.Draft.Text
	}
	return overlay.Dump.Path
}

// FindDumpProblem returns why the dump cannot run, and nothing where it can.
func FindDumpProblem(overlay app.Overlay) string {
	if strings.TrimSpace(readDumpPath(overlay)) == "" {
		return "enter a file path"
	}
	return ""
}

// ReadDumpField writes what was typed into the field under the cursor.
func ReadDumpField(overlay *app.Overlay, written string) {
	fields := BuildDumpFields(*overlay)
	if overlay.Field < 0 || overlay.Field >= len(fields) {
		return
	}
	if fields[overlay.Field].Key == "path" {
		overlay.Dump.Path = written
	}
}

// StepDumpField moves the cursor to another row of the form, and wraps at each end.
func StepDumpField(overlay *app.Overlay, step int) {
	fields := BuildDumpFields(*overlay)
	if len(fields) == 0 {
		return
	}
	if overlay.Draft != nil {
		ReadDumpField(overlay, overlay.Draft.Text)
	}
	overlay.Field = wrap(overlay.Field+step, len(fields))
	value := BuildDumpFields(*overlay)[overlay.Field].Value
	overlay.Draft = app.NewEditorBuffer(value, len(value))
}

// StepDumpChoice takes the next value of the field under the cursor. A field that is typed
// into steps through nothing.
func StepDumpChoice(overlay *app.Overlay, step int) {
	fields := BuildDumpFields(*overlay)
	if overlay.Field < 0 || overlay.Field >= len(fields) {
		return
	}
	field := fields[overlay.Field]
	if len(field.Choices) == 0 {
		return
	}

	at := 0
	for index, choice := range field.Choices {
		if choice == field.Value {
			at = index
		}
	}
	next := field.Choices[wrap(at+step, len(field.Choices))]

	switch field.Key {
	case "content":
		if content, known := dump.FindContentNamed(next); known {
			overlay.Dump.Options.Content = content
		}
	case "drop":
		overlay.Dump.Options.DropsFirst = next == "yes"
	}
}
