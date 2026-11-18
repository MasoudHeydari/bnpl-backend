package checkoutsessionuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// requireSelectedCreditKindForStart ensures Partner SelectCreditKind already
// chose the financing kind. Hosted START cannot invent or override it.
func (uc *UseCase) requireSelectedCreditKindForStart(
	ctx context.Context,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.requireSelectedCreditKindForStart"

	var session model.CheckoutSession

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		sessionQ := uc.checkoutSession.Conn(conn)
		found, findErr := sessionQ.GetByPublicToken(ctx, token)
		if findErr != nil {
			if cerr.IsCode(findErr, cerr.CodeResourceNotFound) {
				return checkoutSessionNotFoundError(findErr)
			}

			return findErr
		}

		if found.CreditKind == "" || !found.CreditKind.IsValid() {
			return validationError("REQUIRED", "credit_kind", "error.credit_kind_required")
		}

		session = found
		return nil
	})
	if err != nil {
		return model.CheckoutSession{}, err
	}

	_ = op
	return session, nil
}
