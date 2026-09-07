package notebook

import (
	"strings"
)

// Write returns the text of the notebook file.
func Write(book Notebook) string {
	var written strings.Builder
	if front := resolveFrontMatter(book); front != "" {
		written.WriteString(FrontMatterMark + "\n" + front + "\n" + FrontMatterMark + "\n")
	}
	for _, cell := range book.Cells {
		if written.Len() > 0 {
			written.WriteString("\n")
		}
		writeCell(&written, cell)
	}
	return written.String()
}

// resolveFrontMatter returns the block as it was read, or a new one for a notebook
// without one.
func resolveFrontMatter(book Notebook) string {
	if held := strings.Trim(book.FrontMatter, "\n"); held != "" {
		return held
	}
	lines := []string{}
	if book.Title != "" {
		lines = append(lines, "title = "+quoteText(book.Title))
	}
	if len(book.Profiles) > 0 {
		quoted := make([]string, 0, len(book.Profiles))
		for _, name := range book.Profiles {
			quoted = append(quoted, quoteText(name))
		}
		lines = append(lines, "profiles = ["+strings.Join(quoted, ", ")+"]")
	}
	if book.Engine != "" {
		lines = append(lines, "engine = "+quoteText(book.Engine))
	}
	policy := book.Run
	if policy.Transaction == "" && policy.OnError == "" {
		policy = DefaultPolicy()
	}
	lines = append(lines, "", "[run]",
		"transaction = "+quoteText(policy.Transaction),
		"on_error = "+quoteText(policy.OnError))
	return strings.Join(lines, "\n")
}

// writeCell writes one cell and the fence around it.
func writeCell(written *strings.Builder, cell Cell) {
	if cell.Kind == CellText {
		written.WriteString(strings.Trim(cell.Source, "\n") + "\n")
		return
	}
	fence := strings.Repeat("`", resolveFenceWidth(cell.Source))
	written.WriteString(fence + buildFenceInfo(cell) + "\n")
	if body := strings.Trim(cell.Source, "\n"); body != "" {
		written.WriteString(body + "\n")
	}
	written.WriteString(fence + "\n")
}

// buildFenceInfo returns the text after the opening fence.
func buildFenceInfo(cell Cell) string {
	if cell.Kind == CellOther {
		return cell.Fence
	}
	parts := []string{string(cell.Kind)}
	if cell.ID != "" {
		parts = append(parts, "id="+cell.ID)
	}
	for _, attr := range cell.Attrs {
		if attr.Name == "id" {
			continue
		}
		parts = append(parts, attr.Name+"="+attr.Value)
	}
	return strings.Join(parts, " ")
}

// resolveFenceWidth returns a fence longer than any fence inside the cell.
func resolveFenceWidth(source string) int {
	widest := 0
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		width := 0
		for width < len(trimmed) && trimmed[width] == '`' {
			width++
		}
		if width > widest {
			widest = width
		}
	}
	if widest < 3 {
		return 3
	}
	return widest + 1
}

// quoteText returns the text as a TOML string.
func quoteText(text string) string {
	replaced := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + replaced.Replace(text) + `"`
}
