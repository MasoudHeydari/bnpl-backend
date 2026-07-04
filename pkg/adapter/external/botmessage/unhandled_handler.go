package botmessage

import (
	"context"
	"fmt"

	corebotmessage "github.com/parstis/bnpl-backend/pkg/core/external/botmessage"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

// UnhandledIncomingMessageHandler is the safe default application callback for
// inbound bot messages until product-specific bot commands are registered.
//
// It intentionally contains no BNPL business logic and returns no user-facing
// reply. A real bot command handler belongs at the inbound adapter/application
// boundary and must call the existing use cases instead of querying repositories
// or reproducing domain rules itself.
//
// For example, a future "my installments" command should resolve the external
// bot identity through an explicit identity-linking flow and then call the
// customer-account use case. The messaging transport must never infer that a
// Telegram/Bale user ID is a BNPL customer ID.
type UnhandledIncomingMessageHandler struct {
	logger corelog.Logger
}

var _ corebotmessage.IncomingMessageHandler = (*UnhandledIncomingMessageHandler)(nil)

// NewUnhandledIncomingMessageHandler creates the default no-business-logic
// inbound callback used by the runtime wiring.
func NewUnhandledIncomingMessageHandler(logger corelog.Logger) (*UnhandledIncomingMessageHandler, error) {
	const op = "botmessage.NewUnhandledIncomingMessageHandler"

	if logger == nil {
		return nil, fmt.Errorf("%s: bot message logger is nil", op)
	}

	return &UnhandledIncomingMessageHandler{logger: logger}, nil
}

// Handle acknowledges an otherwise unsupported inbound text message without
// producing a reply.
//
// This fallback exists so bot transports can be enabled before command-specific
// application handlers are introduced. It should be replaced/decorated by the
// real command router as those flows are implemented.
func (h *UnhandledIncomingMessageHandler) Handle(
	_ context.Context,
	message corebotmessage.IncomingMessage,
) (corebotmessage.HandleResult, error) {
	h.logger.With(
		"integration", message.IntegrationID,
		"external_event_id", message.ExternalEventID,
		"conversation_kind", string(message.Conversation.Kind),
	).Info("bot message received without registered application command handler")

	return corebotmessage.HandleResult{}, nil
}
