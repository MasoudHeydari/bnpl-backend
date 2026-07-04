package financialcorepaymentsessionuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase owns inbound Financial Core installment payment-session processing.
type UseCase struct {
	pool              repo.Pool
	inbox             repo.Inbox
	paymentAttempt    repo.PaymentAttempt
	outbox            repo.Outbox
	idGenerator       identifier.Generator
	fingerprintHasher fingerprint.Hasher
	auditDestination  string
	clock             clock.Clock
}

// New constructs a Financial Core payment-session use case.
func New(
	pool repo.Pool,
	inbox repo.Inbox,
	paymentAttempt repo.PaymentAttempt,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	fingerprintHasher fingerprint.Hasher,
	auditDestination string,
	clk clock.Clock,
) (*UseCase, error) {
	const op = "financialcorepaymentsessionuc.New"

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: identifier generator is required", op)
	}

	if fingerprintHasher == nil {
		return nil, fmt.Errorf("%s: fingerprint generator is required", op)
	}

	if auditDestination == "" {
		return nil, fmt.Errorf("%s: audit destination is required", op)
	}

	return &UseCase{
		pool:              pool,
		inbox:             inbox,
		paymentAttempt:    paymentAttempt,
		outbox:            outbox,
		idGenerator:       idGenerator,
		fingerprintHasher: fingerprintHasher,
		auditDestination:  auditDestination,
		clock:             clk,
	}, nil
}
