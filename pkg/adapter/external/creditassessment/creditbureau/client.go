package creditbureau

import (
	"context"
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	extcreditassessment "github.com/parstis/bnpl-backend/pkg/core/external/creditassessment"
)

var _ extcreditassessment.Client = (*Client)(nil)

// Client is a placeholder credit bureau adapter for future external integrations.
type Client struct {
	breaker circuitbreaker.CircuitBreaker
}

// New constructs a credit bureau credit assessment client.
func New(_ circuitbreaker.CircuitBreaker) (*Client, error) {
	const op = "creditbureau.New"

	return nil, fmt.Errorf("%s: unsupported external credit bureau assessment provider", op)
}

// FetchSignals reports that the external credit bureau integration is not available yet.
func (c *Client) FetchSignals(
	ctx context.Context,
	input extcreditassessment.FetchSignalsInput,
) (extcreditassessment.FetchSignalsResult, error) {
	var result extcreditassessment.FetchSignalsResult
	const op = "creditbureau.Client.FetchSignals"

	err := c.breaker.Execute(ctx, "external.credit_assessment.creditbureau.fetch_signals", func(ctx context.Context) error {
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
	// TODO: Will be implemented in the later releases.
	return extcreditassessment.FetchSignalsResult{}, errors.New(
		"external credit bureau credit assessment integration is not implemented",
	)
}
