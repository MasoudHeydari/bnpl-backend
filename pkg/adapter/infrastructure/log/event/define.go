package event

// Event is a catalog entry implementing corelog.Event.
type Event struct {
	code    string
	message string
}

// Code returns the stable machine-readable event name.
func (e Event) Code() string { return e.code }

// Message returns the human-readable event description.
func (e Event) Message() string { return e.message }

func define(code, message string) Event {
	return Event{code: code, message: message}
}
