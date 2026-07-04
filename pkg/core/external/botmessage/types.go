package botmessage

import "time"

// DestinationKind identifies the semantic kind of a bot conversation.
//
// The value is transport-neutral metadata. Concrete adapters are responsible
// for translating provider-specific chat types into these values.
type DestinationKind string

const (
	// PrivateChatDestination represents a direct/private conversation with a user.
	PrivateChatDestination DestinationKind = "private_chat"

	// GroupDestination represents a group conversation.
	GroupDestination DestinationKind = "group"

	// ChannelDestination represents a broadcast channel.
	ChannelDestination DestinationKind = "channel"
)

// Destination identifies a conversation on an external bot messaging platform.
type Destination struct {
	// IntegrationID identifies the configured bot integration that owns this
	// destination. The value is opaque to core code (for example, an adapter may
	// use values such as "telegram" or "bale").
	//
	// When more than one bot integration is enabled, outbound routing requires
	// this field so provider-specific chat identifiers are never guessed.
	IntegrationID string

	// Address is the provider-specific opaque conversation identifier.
	//
	// It may be a numeric chat identifier, username, channel address, or another
	// value understood by the concrete adapter. Core code must not parse or
	// attach provider-specific meaning to this value.
	Address string

	// Kind describes the semantic conversation type.
	Kind DestinationKind
}

// Participant identifies the external participant that sent an inbound message.
type Participant struct {
	// ID is the provider-specific opaque participant identifier.
	//
	// The identifier is not a BNPL customer identifier. Resolving or linking an
	// external bot identity to a BNPL identity belongs to an explicit application
	// flow and must not be inferred by the messaging adapter.
	ID string
}

// IncomingMessage is the normalized representation of a message received from
// a bot-based messaging platform.
//
// Provider adapters must translate provider payloads into this type before the
// message enters application-facing handling code. Provider DTOs and raw SDK
// objects must not leak into the core layer.
type IncomingMessage struct {
	// IntegrationID identifies the configured bot integration instance that
	// received the message.
	//
	// The value is opaque to core code and namespaces provider-generated IDs.
	// This is useful when multiple bots or multiple providers can emit identical
	// external event/message identifiers.
	IntegrationID string

	// ExternalEventID identifies the provider delivery/update that carried the
	// message. Adapters should populate it whenever the provider exposes a stable
	// event identity so duplicate deliveries can be handled idempotently.
	ExternalEventID string

	// ExternalMessageID is the provider-assigned message identifier.
	ExternalMessageID string

	// Conversation identifies where the message was received.
	Conversation Destination

	// Sender identifies the external participant that sent the message.
	Sender Participant

	// Text contains normalized textual message content.
	Text string

	// OccurredAt is the provider-reported time at which the message occurred.
	OccurredAt time.Time
}

// Reply describes an optional response produced after processing an inbound
// message.
//
// Reply intentionally does not contain a destination. A reply is scoped to the
// conversation of the IncomingMessage that produced it. Proactive messages to
// arbitrary destinations use Sender directly.
type Reply struct {
	// Text is the response body.
	Text string

	// ReplyToIncomingMessage asks the adapter to attach the outgoing message as
	// an explicit reply to the inbound message when the provider supports that
	// capability. When false, the message is still sent to the same conversation
	// but is not required to reference the incoming message explicitly.
	ReplyToIncomingMessage bool
}

// HandleResult describes the messaging actions requested after an inbound
// message has been successfully processed.
type HandleResult struct {
	// Replies contains zero or more responses for the same conversation.
	//
	// An empty slice is a valid successful result and means that no response
	// should be sent to the user.
	Replies []Reply
}

// SendInput contains transport-neutral information required to send a bot
// message.
type SendInput struct {
	// Destination identifies the target conversation.
	Destination Destination

	// Text is the outgoing message body.
	Text string

	// ReplyToExternalMessageID optionally asks the provider to attach this
	// message as a reply to an existing message in Destination.
	ReplyToExternalMessageID string
}

// SendResult contains normalized information returned after a successful send.
type SendResult struct {
	// ExternalMessageID is the identifier assigned to the sent message by the
	// external messaging provider.
	ExternalMessageID string
}

// SendOutcome records the independent result of one item in a batch send.
//
// Err is intentionally per destination because fan-out delivery is not atomic:
// one provider may accept a message while another provider fails.
type SendOutcome struct {
	// Input is the original send request associated with this outcome.
	Input SendInput

	// Result contains the provider message identifier when delivery succeeds.
	Result SendResult

	// Err contains transport-neutral delivery semantics when this destination
	// fails. A nil value means the send succeeded.
	Err error
}
