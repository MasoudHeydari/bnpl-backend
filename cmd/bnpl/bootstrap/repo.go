package bootstrap

import (
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/borrowercreditlimitrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/borroweridentityverificationrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/borrowerprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/decisionrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/healthrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/merchantbranchprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/merchantprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/outboxrp"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Repos holds repository adapters assembled for the application.
type Repos struct {
	Migrations                   repo.Migrations
	Outbox                       repo.Outbox
	Health                       repo.Health
	Decision                     repo.Decision
	BorrowerProfile              repo.BorrowerProfile
	BorrowerIdentityVerification repo.BorrowerIdentityVerification
	BorrowerCreditLimit          repo.BorrowerCreditLimit
	MerchantProfile              repo.MerchantProfile
	MerchantBranchProfile        repo.MerchantBranchProfile
}

// NewRepos constructs the repository adapter set used by App.
func NewRepos() Repos {
	return Repos{
		Migrations:                   migrationsrp.New(),
		Outbox:                       outboxrp.New(),
		Health:                       healthrp.New(),
		Decision:                     decisionrp.New(),
		BorrowerProfile:              borrowerprofilerp.New(),
		BorrowerIdentityVerification: borroweridentityverificationrp.New(),
		BorrowerCreditLimit:          borrowercreditlimitrp.New(),
		MerchantProfile:              merchantprofilerp.New(),
		MerchantBranchProfile:        merchantbranchprofilerp.New(),
	}
}
