package customeraccountuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase loads customer-facing contract and installment query data.
type UseCase struct {
	pool              repo.Pool
	customerProfile   repo.CustomerProfile
	contract          repo.Contract
	installment       repo.Installment
	installmentCharge repo.InstallmentCharge
	paymentAttempt    repo.PaymentAttempt
	clock             clock.Clock
}

// New constructs a customer account use case.
func New(
	pool repo.Pool,
	customerProfile repo.CustomerProfile,
	contract repo.Contract,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	paymentAttempt repo.PaymentAttempt,
	clk clock.Clock,
) (*UseCase, error) {
	const op = "customeraccountuc.New"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if customerProfile == nil {
		return nil, fmt.Errorf("%s: customer profile repository is required", op)
	}

	if contract == nil {
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

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	return &UseCase{
		pool:              pool,
		customerProfile:   customerProfile,
		contract:          contract,
		installment:       installment,
		installmentCharge: installmentCharge,
		paymentAttempt:    paymentAttempt,
		clock:             clk,
	}, nil
}
