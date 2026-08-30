package ui

import "time"

// A terminal never reports a double click, so the client counts the clicks in a row itself.

// doubleClickWindow is how long after a click a second one on the same target is a double
// click.
const doubleClickWindow = 400 * time.Millisecond

// clickCounter counts the clicks in a row on one target.
type clickCounter struct {
	target string
	at     time.Time
	run    int
}

// count returns how many clicks in a row landed on this target, counting from one. A click
// on another target, or a late one, starts at one again.
func (counter *clickCounter) count(target string, at time.Time) int {
	if counter.target == target && at.Sub(counter.at) <= doubleClickWindow {
		counter.run++
	} else {
		counter.run = 1
	}
	counter.target, counter.at = target, at
	return counter.run
}
