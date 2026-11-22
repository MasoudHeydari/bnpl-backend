package checkoutsessionuc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutverificationv1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutverification/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// VerifyByMerchantInput carries an authenticated partner Verify request.
type VerifyByMerchantInput struct {
	CheckoutSessionToken model.CheckoutSessionToken
	ExternalMerchantID   model.ExternalMerchantID
	MerchantOrderID      string
	TerminalID           model.MerchantTerminalID
}

// VerifyByMerchantOutput returns the durable merchant verification receipt.
type VerifyByMerchantOutput struct {
	CheckoutSessionToken model.CheckoutSessionToken
	MerchantOrderID      string
	Status               model.CheckoutSessionStatus
	VerifiedAt           time.Time
}

// VerifyByMerchant records explicit merchant acceptance after durable FC success.
//
// Verify is idempotent for already verified sessions owned by the same merchant
// with a matching merchant_order_id. It never reactivates contracts or credit.
func (uc *UseCase) VerifyByMerchant(
	ctx context.Context,
	input VerifyByMerchantInput,
) (VerifyByMerchantOutput, error) {
	const op = "checkoutsessionuc.UseCase.VerifyByMerchant"

	merchantOrderID := strings.TrimSpace(input.MerchantOrderID)
	if merchantOrderID == "" {
		return VerifyByMerchantOutput{}, cerr.InvalidArgument(fmt.Errorf("%s: merchant_order_id is required", op))
	}

	if input.TerminalID.IsZero() {
		return VerifyByMerchantOutput{}, cerr.InvalidArgument(fmt.Errorf("%s: terminal id is required", op))
	}

	if input.ExternalMerchantID.IsZero() {
		return VerifyByMerchantOutput{}, cerr.InvalidArgument(fmt.Errorf("%s: merchant id is required", op))
	}

	now := uc.clock.Now().UTC()
	var output VerifyByMerchantOutput

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sessionQ := uc.checkoutSession.Tx(tx)
			merchantProfileQ := uc.merchantProfile.Tx(tx)
			contractQ := uc.contract.Tx(tx)

			session, err := sessionQ.LockByPublicToken(ctx, input.CheckoutSessionToken)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					return checkoutSessionNotFoundError(err)
				}

				return err
			}

			profile, err := merchantProfileQ.GetByID(ctx, session.MerchantProfileID)
			if err != nil {
				return err
			}

			if profile.ExternalMerchantID != input.ExternalMerchantID {
				return checkoutSessionNotFoundError(
					fmt.Errorf("%s: checkout session is owned by another merchant", op),
				)
			}

			if session.MerchantOrderID != merchantOrderID {
				return checkoutVerificationConflictError(
					fmt.Errorf("%s: merchant_order_id does not match checkout", op),
				)
			}

			if session.Status == model.VerifiedByMerchantCheckoutSessionStatus {
				verifiedAt := now
				if session.MerchantVerifiedAt != nil {
					verifiedAt = session.MerchantVerifiedAt.UTC()
				} else if session.CompletedAt != nil {
					verifiedAt = session.CompletedAt.UTC()
				}

				output = VerifyByMerchantOutput{
					CheckoutSessionToken: session.PublicToken,
					MerchantOrderID:      session.MerchantOrderID,
					Status:               session.Status,
					VerifiedAt:           verifiedAt,
				}

				return nil
			}

			if session.Status != model.VerifyPendingCheckoutSessionStatus {
				return checkoutVerificationConflictError(
					fmt.Errorf("%s: checkout session status is %q", op, session.Status),
				)
			}

			if session.ContractID == nil || session.ContractID.IsZero() {
				return checkoutVerificationConflictError(
					fmt.Errorf("%s: checkout has no linked contract", op),
				)
			}

			contract, err := contractQ.GetByID(ctx, *session.ContractID)
			if err != nil {
				return err
			}

			if contract.Status != model.ActiveContractStatus {
				return checkoutVerificationConflictError(
					fmt.Errorf("%s: contract status is %q", op, contract.Status),
				)
			}

			if session.VerificationDeadlineAt == nil {
				return cerr.InternalError(fmt.Errorf("%s: verification deadline is missing", op))
			}

			policy := checkoutverificationv1.New()
			if !policy.AllowsVerifyAt(now, *session.VerificationDeadlineAt) {
				return checkoutVerificationDeadlinePassedError(
					fmt.Errorf("%s: verification deadline has passed", op),
				)
			}

			updated, err := sessionQ.MarkVerifiedByMerchant(ctx, session.ID, input.TerminalID, now)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeConflict) {
					replay, getErr := sessionQ.GetByID(ctx, session.ID)
					if getErr != nil {
						return getErr
					}

					if replay.Status == model.VerifiedByMerchantCheckoutSessionStatus {
						verifiedAt := now
						if replay.MerchantVerifiedAt != nil {
							verifiedAt = replay.MerchantVerifiedAt.UTC()
						}

						output = VerifyByMerchantOutput{
							CheckoutSessionToken: replay.PublicToken,
							MerchantOrderID:      replay.MerchantOrderID,
							Status:               replay.Status,
							VerifiedAt:           verifiedAt,
						}

						return nil
					}

					return checkoutVerificationConflictError(err)
				}

				return err
			}

			if err = uc.lifecycle.Enqueue(
				ctx,
				tx,
				updated,
				checkoutlifecycle.EventTypeVerifiedByMerchant,
				now,
			); err != nil {
				return err
			}

			verifiedAt := now
			if updated.MerchantVerifiedAt != nil {
				verifiedAt = updated.MerchantVerifiedAt.UTC()
			}

			output = VerifyByMerchantOutput{
				CheckoutSessionToken: updated.PublicToken,
				MerchantOrderID:      updated.MerchantOrderID,
				Status:               updated.Status,
				VerifiedAt:           verifiedAt,
			}

			return nil
		})
	})
	if err != nil {
		return VerifyByMerchantOutput{}, err
	}

	return output, nil
}
