package noop

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	extcreditassessment "github.com/parstis/bnpl-backend/pkg/core/external/creditassessment"
)

const noopProviderName = "noop"

var _ extcreditassessment.Client = (*Client)(nil)

// Client returns an empty credit signal result without calling an external provider.
type Client struct {
	breaker circuitbreaker.CircuitBreaker
	clock   clock.Clock
}

// New constructs a noop credit assessment client.
func New(breaker circuitbreaker.CircuitBreaker, clk clock.Clock) (*Client, error) {
	const op = "noop.New"

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	return &Client{breaker: breaker, clock: clk}, nil
}

// FetchSignals returns an empty signal set for local development and tests.
func (c *Client) FetchSignals(
	ctx context.Context,
	input extcreditassessment.FetchSignalsInput,
) (extcreditassessment.FetchSignalsResult, error) {
	var result extcreditassessment.FetchSignalsResult
	const op = "noop.Client.FetchSignals"

	err := c.breaker.Execute(ctx, "external.credit_assessment.noop.fetch_signals", func(ctx context.Context) error {
		var fetchErr error

		result, fetchErr = c.fetchSignals(ctx, input)

		return fetchErr
	})
	if err != nil {
		return extcreditassessment.FetchSignalsResult{}, fmt.Errorf("%s: credit assessment fetch signals: %w", op, err)
	}

	return result, nil
}

func (c *Client) fetchSignals(
	_ context.Context,
	_ extcreditassessment.FetchSignalsInput,
) (extcreditassessment.FetchSignalsResult, error) {
	now := c.clock.Now().UTC()

	return extcreditassessment.FetchSignalsResult{
		Provider:  noopProviderName,
		FetchedAt: now,
		ExpiresAt: now,
		Signals:   []extcreditassessment.Signal{},
	}, nil
}
