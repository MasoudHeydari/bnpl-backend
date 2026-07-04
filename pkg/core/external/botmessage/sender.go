package botmessage

import "context"

// Sender sends messages through one configured bot messaging integration.
//
// Concrete implementations belong to adapter packages and may use Telegram,
// Bale, or another provider. Provider-specific request/response types must not
// leak through this interface.
type Sender interface {
	// Send delivers one message to the requested destination.
	Send(ctx context.Context, input SendInput) (SendResult, error)
}

// BatchSender fans one logical operation out to multiple independently routed
// bot destinations.
//
// Batch delivery is deliberately non-atomic. Implementations must attempt every
// input and return one outcome per item so a failure on one provider does not
// suppress delivery to another provider.
type BatchSender interface {
	// SendBatch attempts every input and preserves input order in the outcomes.
	SendBatch(ctx context.Context, inputs []SendInput) []SendOutcome
}
