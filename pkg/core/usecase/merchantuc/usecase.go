package merchantuc

import (
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase reads merchant profiles, branches, and sales.
type UseCase struct {
	pool            repo.Pool
	merchantProfile repo.MerchantProfile
	branchProfile   repo.MerchantBranchProfile
	contract        repo.Contract
}

// New constructs a merchant query use case.
func New(
	pool repo.Pool,
	merchantProfile repo.MerchantProfile,
	branchProfile repo.MerchantBranchProfile,
	contract repo.Contract,
) *UseCase {
	return &UseCase{
		pool:            pool,
		merchantProfile: merchantProfile,
		branchProfile:   branchProfile,
		contract:        contract,
	}
}
