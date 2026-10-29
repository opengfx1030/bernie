package bench

import "io"

// The kinds of a progress event.
const (
	// EventVisit is the start of a visit of an A/B run.
	EventVisit = "visit"
	// EventGroup is the start of a group of a suite run.
	EventGroup = "group"
	// EventCell is a cell of the run that finished.
	EventCell = "cell"
)

// Event is a typed progress event of a run. The kind is one of the
// constants above. A run sends its events to the progress writer when the
// writer takes events; the progress lines stay the text of the run.
type Event struct {
	// Kind is the kind of the event.
	Kind string `json:"kind"`
	// Index is the number of the visit or of the cell, starting at one.
	Index int `json:"index,omitempty"`
	// Total is the number of visits, of cells or of groups of the run.
	Total int `json:"total,omitempty"`
	// Arm is the arm of a visit: base or cand.
	Arm string `json:"arm,omitempty"`
	// Group is the name of the group of a suite run.
	Group string `json:"group,omitempty"`
	// Level is the concurrency of a cell.
	Level int `json:"level,omitempty"`
}

// EventWriter is a progress writer that also takes the typed events of a
// run. A writer that does not implement it gets the progress lines only.
type EventWriter interface {
	io.Writer
	Event(Event)
}

// SendEvent sends a typed event to a progress writer. A writer that does
// not take events gets nothing.
func SendEvent(w io.Writer, e Event) {
	if ew, ok := w.(EventWriter); ok {
		ew.Event(e)
	}
}
