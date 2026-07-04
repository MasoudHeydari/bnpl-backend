package customeraccountuc

import (
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
}

// New constructs a customer account use case.
func New(
	pool repo.Pool,
	customerProfile repo.CustomerProfile,
	contract repo.Contract,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	paymentAttempt repo.PaymentAttempt,
) *UseCase {
	return &UseCase{
		pool:              pool,
		customerProfile:   customerProfile,
		contract:          contract,
		installment:       installment,
		installmentCharge: installmentCharge,
		paymentAttempt:    paymentAttempt,
	}
}
