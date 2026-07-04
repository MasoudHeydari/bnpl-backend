package financialcorepaymentresultuc

import (
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	coremetrics "github.com/parstis/bnpl-backend/pkg/core/metrics"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

// UseCase owns inbound Financial Core payment-result processing.
type UseCase struct {
	pool                                 repo.Pool
	contractUC                           *contractuc.UseCase
	checkoutSessionUC                    checkoutSessionPaymentHook
	checkoutPaymentClient                extcheckoutpayment.Client
	inbox                                repo.Inbox
	outbox                               repo.Outbox
	paymentAttempt                       repo.PaymentAttempt
	contract                             repo.Contract
	installment                          repo.Installment
	installmentCharge                    repo.InstallmentCharge
	creditAccount                        repo.CreditAccount
	idGenerator                          identifier.Generator
	fingerprintHasher                    fingerprint.Hasher
	paymentApplicationResultsDestination string
	auditDestination                     string
	clock                                clock.Clock
	paymentResultProcessingGracePeriod   time.Duration
	businessMetrics                      coremetrics.BusinessMetrics
}

// New constructs a Financial Core payment-result use case.
func New(
	pool repo.Pool,
	contractUC *contractuc.UseCase,
	checkoutSessionUC checkoutSessionPaymentHook,
	checkoutPaymentClient extcheckoutpayment.Client,
	inbox repo.Inbox,
	outbox repo.Outbox,
	paymentAttempt repo.PaymentAttempt,
	contractRepo repo.Contract,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	creditAccount repo.CreditAccount,
	idGenerator identifier.Generator,
	fingerprintHasher fingerprint.Hasher,
	paymentApplicationResultsDestination string,
	auditDestination string,
	clk clock.Clock,
	paymentResultProcessingGracePeriod time.Duration,
	businessMetrics coremetrics.BusinessMetrics,
) (*UseCase, error) {
	const op = "financialcorepaymentresultuc.New"

	if contractUC == nil {
		return nil, fmt.Errorf("%s: contract use case is required", op)
	}

	if checkoutSessionUC == nil {
		return nil, fmt.Errorf("%s: checkout session use case is required", op)
	}

	if checkoutPaymentClient == nil {
		return nil, fmt.Errorf("%s: checkout payment client is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: id generator is required", op)
	}

	if fingerprintHasher == nil {
		return nil, fmt.Errorf("%s: fingerprint generator is required", op)
	}

	if paymentApplicationResultsDestination == "" {
		return nil, fmt.Errorf("%s: payment application results destination is required", op)
	}

	if auditDestination == "" {
		return nil, fmt.Errorf("%s: audit destination is required", op)
	}

	if paymentResultProcessingGracePeriod < 0 {
		return nil, fmt.Errorf("%s: payment result processing grace period must not be negative", op)
	}

	if paymentResultProcessingGracePeriod > model.MaxPaymentResultProcessingGracePeriod {
		return nil, fmt.Errorf(
			"%s: payment result processing grace period must not exceed %s", op, model.MaxPaymentResultProcessingGracePeriod,
		)
	}

	if businessMetrics == nil {
		return nil, fmt.Errorf("%s: business metrics is required", op)
	}

	return &UseCase{
		pool:                                 pool,
		contractUC:                           contractUC,
		checkoutSessionUC:                    checkoutSessionUC,
		checkoutPaymentClient:                checkoutPaymentClient,
		inbox:                                inbox,
		outbox:                               outbox,
		paymentAttempt:                       paymentAttempt,
		contract:                             contractRepo,
		installment:                          installment,
		installmentCharge:                    installmentCharge,
		creditAccount:                        creditAccount,
		idGenerator:                          idGenerator,
		fingerprintHasher:                    fingerprintHasher,
		paymentApplicationResultsDestination: paymentApplicationResultsDestination,
		auditDestination:                     auditDestination,
		clock:                                clk,
		paymentResultProcessingGracePeriod:   paymentResultProcessingGracePeriod,
		businessMetrics:                      businessMetrics,
	}, nil
}
