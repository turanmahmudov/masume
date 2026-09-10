package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/present"
)

// A run that writes rows or statements reports how far it has come through a channel, which
// the draw loop reads between frames. The card draws a bar where the size of the run is
// known, and the counts alone where it is not.

// progressRoom is how many reports the channel holds. A run that is faster than the frames
// drops what the screen would never draw, because a report is a level and not a step.
const progressRoom = 4

// progressMsg carries the last report the run made.
type progressMsg struct {
	ConnectionID int
	Kind         app.OverlayKind
	Progress     app.Progress
	// Source is the channel it arrived on, so the next wait reads the same run to its end
	// and never the channel of a later one.
	Source chan app.Progress
}

// startProgress returns the channel a run reports on and the command that reads it.
func startProgress(
	connectionID int, kind app.OverlayKind,
) (chan app.Progress, tea.Cmd) {
	updates := make(chan app.Progress, progressRoom)
	return updates, waitForProgress(connectionID, kind, updates)
}

// waitForProgress waits for one report and takes the last of the reports behind it.
func waitForProgress(
	connectionID int, kind app.OverlayKind, updates chan app.Progress,
) tea.Cmd {
	return func() tea.Msg {
		held, open := <-updates
		if !open {
			return nil
		}
		// Only the last report is drawn, so the ones behind it are read and dropped.
		for {
			select {
			case next, stillOpen := <-updates:
				if !stillOpen {
					return progressMsg{
						ConnectionID: connectionID, Kind: kind, Progress: held,
					}
				}
				held = next
			default:
				return progressMsg{
					ConnectionID: connectionID, Kind: kind,
					Progress: held, Source: updates,
				}
			}
		}
	}
}

// sendProgress reports how far a run has come. A full channel keeps the report the screen
// has not read yet, and this one is dropped: the run never waits for the screen.
func sendProgress(updates chan app.Progress, held app.Progress) {
	select {
	case updates <- held:
	default:
	}
}

// closeProgress ends the reports of a run, so the wait of the draw loop ends with it. A
// channel that was never made belongs to a run that reports nothing.
func closeProgress(updates chan app.Progress) {
	if updates != nil {
		close(updates)
	}
}

// readProgress draws the report on the card it belongs to, and waits for the next one.
func (model *Model) readProgress(answered progressMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found || connection.Overlay.Kind != answered.Kind {
		return model, nil
	}
	switch answered.Kind {
	case app.OverlayImport:
		connection.Overlay.Import.Progress = answered.Progress
	case app.OverlayDump:
		connection.Overlay.Dump.Progress = answered.Progress
	}
	if answered.Source == nil {
		return model, nil
	}
	return model, waitForProgress(answered.ConnectionID, answered.Kind, answered.Source)
}

// progressBarWidth is how many cells the bar of a card takes.
const progressBarWidth = 24

// renderProgress draws one line: the bar where the size of the run is known, then the counts
// and the part that runs now.
func (model *Model) renderProgress(held app.Progress, width int) string {
	if !held.IsStarted() {
		return ""
	}
	written := describeProgressCount(held)
	if held.Detail != "" {
		written += " · " + held.Detail
	}
	if !held.HoldsBar() {
		return model.styles.Muted().Render(present.TruncateText(written, width))
	}

	bar := present.BuildMeter(float64(held.Done), float64(held.Total), progressBarWidth)
	return model.styles.Accent().Render(bar) + " " +
		model.styles.Muted().Render(present.TruncateText(
			written, max(width-progressBarWidth-1, 8)))
}

// describeProgressCount writes the parts done, and the parts expected where they are known.
func describeProgressCount(held app.Progress) string {
	written := present.FormatCount(held.Done)
	if held.HoldsBar() {
		written += " of " + present.FormatCount(held.Total)
	}
	return written + " " + held.Label
}
