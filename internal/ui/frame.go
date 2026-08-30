package ui

import (
	"time"
)

// viewMarks is everything laid over the frame once it is drawn: what the pointer stands on,
// the cells a drag covers, and the key a press lit. A frame that is held is painted again only
// where one of these changed, so a move that marks the same thing costs nothing at all.
type viewMarks struct {
	hover     hoverTarget
	selection screenSelection
	pressed   buttonHit
	lit       bool
}

type screenFrame struct {
	// The frame as it was last drawn, so a copy reads the cells the drag covered.
	text string
	// The frame as it was last handed to the terminal, with the marks of the pointer and
	// of the drag on it, so a move that changes nothing costs no frame.
	shown string
	// True while the frame on screen still stands, because the pointer moved and stayed on
	// the same thing.
	held bool
	// The marks that were last laid over the frame, so a frame that is held is painted
	// again only where one of them changed.
	marks viewMarks
	// Where the pointer stands, and what it stands on, so the frame marks it.
	hover              hoverTarget
	pointerX, pointerY int
	// The key a press last landed on, and when, so the press is answered on the frame.
	pressed   buttonHit
	pressedAt time.Time
}

func (frame *screenFrame) followPointer(x, y int) {
	frame.pointerX, frame.pointerY = x, y
}

func (frame *screenFrame) flashKey(held buttonHit) {
	frame.pressed, frame.pressedAt = held, time.Now()
}

func (frame *screenFrame) isFlashing() bool {
	return !frame.pressedAt.IsZero() && time.Since(frame.pressedAt) < keyFlashWait
}

func (frame *screenFrame) needsDraw() bool {
	return !frame.held || frame.text == ""
}

func (frame *screenFrame) needsPaint(marks viewMarks) bool {
	return frame.shown == "" || marks != frame.marks
}

func (frame *screenFrame) keepMarks(marks viewMarks) {
	frame.hover, frame.marks = marks.hover, marks
}
