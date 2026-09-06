package query

import (
	"regexp"
	"strings"
)

// Model replies contain prose and proposed statements in code blocks.

// statementFences is the set of code block language tags recognized as statements.
const statementFences = "sql|js|javascript|mongodb|mongosh"

// fencedBlock matches one fenced block, with or without the language named.
var fencedBlock = regexp.MustCompile(
	"(?is)```(?:" + statementFences + ")?[ \t]*\r?\n(.*?)```")

// The kinds of part a reply holds.
const (
	SegmentText = "text"
	SegmentSQL  = "sql"
)

// MessageSegment is one part of a reply: text, or a statement.
type MessageSegment struct {
	Kind    string
	Content string
}

// SplitMessageSegments returns a reply as its parts, in the order they were written.
func SplitMessageSegments(message string) []MessageSegment {
	segments := []MessageSegment{}
	cursor := 0

	for _, found := range fencedBlock.FindAllStringSubmatchIndex(message, -1) {
		start, end := found[0], found[1]
		if start > cursor {
			segments = append(segments,
				MessageSegment{Kind: SegmentText, Content: message[cursor:start]})
		}
		block := ""
		if found[2] != -1 {
			block = strings.TrimSpace(message[found[2]:found[3]])
		}
		segments = append(segments, MessageSegment{Kind: SegmentSQL, Content: block})
		cursor = end
	}
	if cursor < len(message) {
		segments = append(segments,
			MessageSegment{Kind: SegmentText, Content: message[cursor:]})
	}
	return segments
}

// FindSQLBlock returns the statement the assistant proposed, and whether it wrote one.
func FindSQLBlock(reply string) (string, bool) {
	for _, segment := range SplitMessageSegments(reply) {
		if segment.Kind == SegmentSQL && segment.Content != "" {
			return segment.Content, true
		}
	}
	return "", false
}
