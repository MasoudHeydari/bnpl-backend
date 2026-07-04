package botmessage

import "context"

// IncomingMessageHandler processes a normalized inbound bot message.
//
// Implementations are application-facing orchestration boundaries, not places
// for BNPL business rules. They may interpret conversational intent, validate
// presentation-level input, call the appropriate core use case, and map the
// use-case output to zero or more Reply values. Domain decisions and data access
// must remain in the corresponding use cases and repositories.
//
// For example, a message meaning "show my installments" should be translated
// into a call to the installment/customer-facing use case that already owns that
// behavior; the bot handler must not reimplement installment selection rules or
// query persistence directly.
//
// Returning a HandleResult with no Replies means the message was handled
// successfully and intentionally requires no response. Returning an error means
// processing did not complete successfully; the inbound adapter decides how its
// delivery mechanism acknowledges, retries, or reports that failure.
type IncomingMessageHandler interface {
	// Handle processes one normalized incoming message.
	Handle(ctx context.Context, message IncomingMessage) (HandleResult, error)
}
