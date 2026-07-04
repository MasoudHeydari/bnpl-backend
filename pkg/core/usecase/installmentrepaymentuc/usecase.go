package installmentrepaymentuc

import (
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// DefaultPaymentSessionTTL is the gateway payment-session lifetime requested for
// a newly created installment repayment PaymentAttempt. Requested expiry is
// attempt.CreatedAt + TTL so retries of the same logical attempt stay stable.
//
// This duration is an immutable policy constant for in-flight attempts: do not
// load it from runtime configuration in a way that would change expires_at for
// an existing PaymentAttempt while reusing its idempotency key. Aligned in
// duration with the checkout first-installment payment window, but owned by
// installment repayment (not installment.DueAt).
const DefaultPaymentSessionTTL = 15 * time.Minute

// UseCase orchestrates customer-initiated installment repayment.
type UseCase struct {
	pool                  repo.Pool
	customerProfile       repo.CustomerProfile
	merchantProfile       repo.MerchantProfile
	contract              repo.Contract
	installment           repo.Installment
	installmentCharge     repo.InstallmentCharge
	paymentAttempt        repo.PaymentAttempt
	outbox                repo.Outbox
	checkoutPaymentClient extcheckoutpayment.Client
	clock                 clock.Clock
	logger                log.Logger
	auditDestination      string
	idGenerator           identifier.Generator
	paymentReturnURL      string
	paymentSessionTTL     time.Duration
}

// New constructs an installment repayment use case.
func New(
	pool repo.Pool,
	customerProfile repo.CustomerProfile,
	merchantProfile repo.MerchantProfile,
	contractRepo repo.Contract,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	paymentAttempt repo.PaymentAttempt,
	outbox repo.Outbox,
	checkoutPaymentClient extcheckoutpayment.Client,
	clk clock.Clock,
	logger log.Logger,
	auditDestination string,
	idGenerator identifier.Generator,
	paymentReturnURL string,
) (*UseCase, error) {
	const op = "installmentrepaymentuc.New"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if customerProfile == nil {
		return nil, fmt.Errorf("%s: customer profile repository is required", op)
	}

	if merchantProfile == nil {
		return nil, fmt.Errorf("%s: merchant profile repository is required", op)
	}

	if contractRepo == nil {
		return nil, fmt.Errorf("%s: contract repository is required", op)
	}

	if installment == nil {
		return nil, fmt.Errorf("%s: installment repository is required", op)
	}

	if installmentCharge == nil {
		return nil, fmt.Errorf("%s: installment charge repository is required", op)
	}

	if paymentAttempt == nil {
		return nil, fmt.Errorf("%s: payment attempt repository is required", op)
	}

	if outbox == nil {
		return nil, fmt.Errorf("%s: outbox repository is required", op)
	}

	if checkoutPaymentClient == nil {
		return nil, fmt.Errorf("%s: checkout payment client is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if auditDestination == "" {
		return nil, fmt.Errorf("%s: audit destination is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: id generator is required", op)
	}

	if strings.TrimSpace(paymentReturnURL) == "" {
		return nil, fmt.Errorf("%s: payment return url is required", op)
	}

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &UseCase{
		pool:                  pool,
		customerProfile:       customerProfile,
		merchantProfile:       merchantProfile,
		contract:              contractRepo,
		installment:           installment,
		installmentCharge:     installmentCharge,
		paymentAttempt:        paymentAttempt,
		outbox:                outbox,
		checkoutPaymentClient: checkoutPaymentClient,
		clock:                 clk,
		logger:                logger,
		auditDestination:      auditDestination,
		idGenerator:           idGenerator,
		paymentReturnURL:      strings.TrimSpace(paymentReturnURL),
		paymentSessionTTL:     DefaultPaymentSessionTTL,
	}, nil
}
