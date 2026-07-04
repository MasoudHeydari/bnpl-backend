package financialcorepaymentreconciliationuc

import (
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

// UseCase owns inbound Financial Core installment payment reconciliation results.
type UseCase struct {
	pool                                      repo.Pool
	paymentResult                             *financialcorepaymentresultuc.UseCase
	inbox                                     repo.Inbox
	outbox                                    repo.Outbox
	paymentAttempt                            repo.PaymentAttempt
	contract                                  repo.Contract
	installment                               repo.Installment
	installmentCharge                         repo.InstallmentCharge
	creditAccount                             repo.CreditAccount
	fingerprintHasher                         fingerprint.Hasher
	idGenerator                               identifier.Generator
	reconciliationAcknowledgementsDestination string
	auditDestination                          string
	retryBaseDelay                            time.Duration
	retryMaxDelay                             time.Duration
	clock                                     clock.Clock
}

// New constructs a Financial Core payment reconciliation result use case.
func New(
	pool repo.Pool,
	paymentResult *financialcorepaymentresultuc.UseCase,
	inbox repo.Inbox,
	outbox repo.Outbox,
	paymentAttempt repo.PaymentAttempt,
	contractRepo repo.Contract,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	creditAccount repo.CreditAccount,
	fingerprintHasher fingerprint.Hasher,
	idGenerator identifier.Generator,
	installmentPaymentReconciliationAcknowledgement string,
	auditDestination string,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	clk clock.Clock,
) (*UseCase, error) {
	const op = "financialcorepaymentreconciliationuc.New"

	if paymentResult == nil {
		return nil, fmt.Errorf("%s: payment result use case is required", op)
	}

	if fingerprintHasher == nil {
		return nil, fmt.Errorf("%s: fingerprint generator is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: id generator is required", op)
	}

	if installmentPaymentReconciliationAcknowledgement == "" {
		return nil, fmt.Errorf("%s: reconciliation acknowledgements destination is required", op)
	}

	if auditDestination == "" {
		return nil, fmt.Errorf("%s: audit destination is required", op)
	}

	if retryBaseDelay <= 0 {
		return nil, fmt.Errorf("%s: retry base delay must be positive", op)
	}

	if retryMaxDelay < retryBaseDelay {
		return nil, fmt.Errorf("%s: retry max delay must be greater than or equal to retry base delay", op)
	}

	return &UseCase{
		pool:              pool,
		paymentResult:     paymentResult,
		inbox:             inbox,
		outbox:            outbox,
		paymentAttempt:    paymentAttempt,
		contract:          contractRepo,
		installment:       installment,
		installmentCharge: installmentCharge,
		creditAccount:     creditAccount,
		fingerprintHasher: fingerprintHasher,
		idGenerator:       idGenerator,
		reconciliationAcknowledgementsDestination: installmentPaymentReconciliationAcknowledgement,
		auditDestination: auditDestination,
		retryBaseDelay:   retryBaseDelay,
		retryMaxDelay:    retryMaxDelay,
		clock:            clk,
	}, nil
}
