package log

// Event is a structured, machine-searchable log checkpoint.
//
// Business catalog events live in the adapter layer. Core may emit Text
// events for operational messages that are not part of the BNPL catalog.
type Event interface {
	// Code returns the stable machine-readable event name
	// (for example "checkout.session.created").
	Code() string

	// Message returns a short human-readable description of the event.
	Message() string
}

// textEvent is a Core-safe Event for operational logging outside the adapter
// business catalog (migrations, low-level core infrastructure).
type textEvent struct {
	code    string
	message string
}

// Text returns an Event with an explicit code and message.
func Text(code, message string) Event {
	return textEvent{code: code, message: message}
}

// Msg returns an Event whose code and message are both message.
//
// Prefer catalog Events in Adapter for business checkpoints. Msg is for
// operational records that Core or infrastructure emit without a catalog entry.
func Msg(message string) Event {
	return textEvent{code: message, message: message}
}

func (e textEvent) Code() string {
	return e.code
}

func (e textEvent) Message() string {
	return e.message
}
