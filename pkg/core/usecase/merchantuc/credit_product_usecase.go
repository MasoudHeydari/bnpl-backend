package merchantuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// CreditProductUseCase administers which BNPL credit products a merchant may
// offer. It is an administration surface: callers must be authorized by the
// transport layer (permission.MerchantsCreditProductsUpdateAny for writes and
// permission.MerchantsReadAny for reads) before invoking it.
type CreditProductUseCase struct {
	pool             repo.Pool
	merchantProfile  repo.MerchantProfile
	outbox           repo.Outbox
	clock            clock.Clock
	idGenerator      identifier.Generator
	auditDestination string
}

// NewCreditProduct constructs the merchant credit product administration use case.
func NewCreditProduct(
	pool repo.Pool,
	merchantProfile repo.MerchantProfile,
	outbox repo.Outbox,
	clk clock.Clock,
	idGenerator identifier.Generator,
	auditDestination string,
) (*CreditProductUseCase, error) {
	const op = "merchantuc.NewCreditProduct"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if merchantProfile == nil {
		return nil, fmt.Errorf("%s: merchant profile repository is required", op)
	}

	if outbox == nil {
		return nil, fmt.Errorf("%s: outbox repository is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: identifier generator is required", op)
	}

	if auditDestination == "" {
		return nil, fmt.Errorf("%s: audit destination is required", op)
	}

	return &CreditProductUseCase{
		pool:             pool,
		merchantProfile:  merchantProfile,
		outbox:           outbox,
		clock:            clk,
		idGenerator:      idGenerator,
		auditDestination: auditDestination,
	}, nil
}
