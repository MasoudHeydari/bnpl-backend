package botmessage

import (
	"context"
	"errors"
	"fmt"
	"sync"

	corebotmessage "github.com/parstis/bnpl-backend/pkg/core/external/botmessage"
)

var (
	// ErrIntegrationNotConfigured indicates that an outbound destination refers
	// to a bot integration that is not enabled in the current process.
	ErrIntegrationNotConfigured = errors.New("bot message integration is not configured")

	// ErrIntegrationIDRequired indicates that more than one integration is
	// enabled and the destination did not identify which one should be used.
	ErrIntegrationIDRequired = errors.New("bot message integration id is required")
)

// Integration is one runnable bot provider adapter.
//
// Run owns inbound listening for the provider (for example long polling) and
// delegates normalized messages to handler. Sender owns outbound delivery.
// Neither method may contain BNPL business rules.
type Integration interface {
	corebotmessage.Sender

	// ID returns the stable adapter routing identifier.
	ID() IntegrationID

	// Run receives provider updates until ctx is canceled. A non-nil return
	// while ctx is active is treated as an unexpected runtime failure.
	Run(ctx context.Context, handler corebotmessage.IncomingMessageHandler) error
}

// Manager coordinates all bot integrations enabled for one process.
//
// Multiple real integrations may coexist. In particular Telegram and Bale can
// both listen concurrently while outbound Send calls are routed by
// Destination.IntegrationID.
type Manager struct {
	integrations map[IntegrationID]Integration
	ordered      []Integration
}

var _ corebotmessage.Sender = (*Manager)(nil)
var _ corebotmessage.BatchSender = (*Manager)(nil)

// NewManager creates a bot integration manager.
func NewManager(integrations ...Integration) (*Manager, error) {
	const op = "botmessage.NewManager"

	if len(integrations) == 0 {
		return nil, fmt.Errorf("%s: bot message manager requires at least one integration", op)
	}

	manager := &Manager{
		integrations: make(map[IntegrationID]Integration, len(integrations)),
		ordered:      make([]Integration, 0, len(integrations)),
	}

	for _, integration := range integrations {
		if integration == nil {
			return nil, fmt.Errorf("%s: bot message manager integration is nil", op)
		}

		id := integration.ID()
		if id == "" {
			return nil, fmt.Errorf("%s: bot message integration id is empty", op)
		}

		if _, exists := manager.integrations[id]; exists {
			return nil, fmt.Errorf("%s: duplicate bot message integration id %q", op, id)
		}

		manager.integrations[id] = integration
		manager.ordered = append(manager.ordered, integration)
	}

	return manager, nil
}

// Send routes one outbound message to the integration selected by the
// destination. If exactly one integration is enabled, an empty IntegrationID is
// accepted and resolves to that integration for convenience.
func (m *Manager) Send(ctx context.Context, input corebotmessage.SendInput) (corebotmessage.SendResult, error) {
	integration, err := m.resolve(input.Destination.IntegrationID)
	if err != nil {
		return corebotmessage.SendResult{}, corebotmessage.PermanentError(err)
	}

	return integration.Send(ctx, input)
}

// SendBatch attempts every outbound input independently and preserves input
// order in the returned outcomes.
//
// A failure for Telegram does not prevent the same logical notification from
// being attempted for Bale, or vice versa. Callers can therefore fan out one
// logical message by supplying one destination per integration.
func (m *Manager) SendBatch(ctx context.Context, inputs []corebotmessage.SendInput) []corebotmessage.SendOutcome {
	outcomes := make([]corebotmessage.SendOutcome, len(inputs))
	for i, input := range inputs {
		result, err := m.Send(ctx, input)
		outcomes[i] = corebotmessage.SendOutcome{
			Input:  input,
			Result: result,
			Err:    err,
		}
	}

	return outcomes
}

// Run starts every configured provider listener concurrently and blocks until
// ctx is canceled or one integration exits unexpectedly.
//
// If one provider fails unexpectedly, the manager cancels the sibling
// listeners and returns the failure so the process supervisor can drain the
// application instead of silently running with a partially dead bot surface.
func (m *Manager) Run(ctx context.Context, handler corebotmessage.IncomingMessageHandler) error {
	const op = "botmessage.Manager.Run"

	if handler == nil {
		return fmt.Errorf("%s: bot message incoming handler is nil", op)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		id  IntegrationID
		err error
	}

	results := make(chan result, len(m.ordered))
	var wg sync.WaitGroup
	for _, integration := range m.ordered {
		integration := integration
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- result{id: integration.ID(), err: integration.Run(runCtx, handler)}
		}()
	}

	first := <-results
	if ctx.Err() != nil {
		cancel()
		wg.Wait()
		return nil
	}

	if first.err == nil {
		first.err = errors.New("listener stopped without context cancellation")
	}

	cancel()
	wg.Wait()

	return fmt.Errorf("%s: bot message integration %q stopped: %w", op, first.id, first.err)
}

func (m *Manager) resolve(rawID string) (Integration, error) {
	const op = "botmessage.Manager.resolve"

	if rawID == "" {
		if len(m.ordered) == 1 {
			return m.ordered[0], nil
		}

		return nil, ErrIntegrationIDRequired
	}

	integration, ok := m.integrations[IntegrationID(rawID)]
	if !ok {
		return nil, fmt.Errorf("%s: %w: %q", op, ErrIntegrationNotConfigured, rawID)
	}

	return integration, nil
}
