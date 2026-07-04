package checkoutsessionuc

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
)

// RequestExternalOTPInput requests an SMS OTP for hosted checkout — either the
// initial challenge after Start prepared an offer, or a payment-continuation
// challenge after a linked Contract still needs customer payment.
type RequestExternalOTPInput struct {
	Token                model.CheckoutSessionToken
	EvaluationGeneration int
}

func (i RequestExternalOTPInput) validate() error {
	if err := validateCheckoutSessionToken(i.Token); err != nil {
		return err
	}

	if i.EvaluationGeneration < 1 {
		return validationError(
			"REQUIRED",
			"evaluation_generation",
			"error.evaluation_generation_required",
		)
	}

	return nil
}

// RequestExternalOTPOutput is the privacy-safe OTP issuance result.
type RequestExternalOTPOutput struct {
	Token                model.CheckoutSessionToken
	Status               model.CheckoutSessionStatus
	EvaluationGeneration int
	OTPSent              bool
	OTPExpiresAt         *time.Time
	OTPNextIssueAt       *time.Time
	OTPAvailability      string
}

// RequestExternalOTP is the single public OTP issuance command for Hosted
// Checkout. It never prepares offers, never creates financial operations, and
// never calls Financial Core.
//
// Flow:
//  1. Validate token + evaluation_generation against the persisted session.
//  2. Load trusted merchant display name (DB read; no network).
//  3. Delegate issuance to the generic OTP use case. The hosted authorizer
//     locks in canonical OTP×Saga×FC order (Saga → Contract → PA → CS for
//     continuation; CS only for open), then OTP locks the challenge and
//     rate-limit rows.
func (uc *UseCase) RequestExternalOTP(
	ctx context.Context,
	input RequestExternalOTPInput,
) (RequestExternalOTPOutput, error) {
	if err := input.validate(); err != nil {
		return RequestExternalOTPOutput{}, err
	}

	session, err := uc.loadExternalSessionForOTPRequest(ctx, input)
	if err != nil {
		return RequestExternalOTPOutput{}, err
	}

	merchantName, err := uc.merchantDisplayName(ctx, session.MerchantProfileID)
	if err != nil {
		return RequestExternalOTPOutput{}, err
	}

	issued, err := uc.otp.Request(ctx, otp.RequestInput{
		Purpose:        model.OTPPurposeHostedCheckoutConfirm,
		SubjectKey:     session.ID.String(),
		ContextVersion: strconv.Itoa(input.EvaluationGeneration),
		Authorizer: &hostedCheckoutOTPAuthorizer{
			uc:                 uc,
			token:              input.Token,
			expectedGeneration: input.EvaluationGeneration,
			merchantName:       merchantName,
			templateID:         uc.otpTemplateID,
		},
	})
	if err != nil {
		return RequestExternalOTPOutput{}, err
	}

	return RequestExternalOTPOutput{
		Token:                session.PublicToken,
		Status:               session.Status,
		EvaluationGeneration: session.EvaluationGeneration,
		OTPSent:              issued.OTPSent,
		OTPExpiresAt:         issued.OTPExpiresAt,
		OTPNextIssueAt:       issued.OTPNextIssueAt,
		OTPAvailability:      issued.Availability,
	}, nil
}

func (uc *UseCase) loadExternalSessionForOTPRequest(
	ctx context.Context,
	input RequestExternalOTPInput,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionuc.UseCase.loadExternalSessionForOTPRequest"

	var session model.CheckoutSession

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		sessionQ := uc.checkoutSession.Conn(conn)
		found, err := sessionQ.GetByPublicToken(ctx, input.Token)
		if err != nil {
			if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				return checkoutSessionNotFoundError(err)
			}

			return err
		}

		if found.Origin != model.ExternalPartnerCheckoutSessionOrigin {
			return checkoutSessionNotFoundError(
				fmt.Errorf("%s: checkout session origin is %q", op, found.Origin),
			)
		}

		if found.EvaluationGeneration != input.EvaluationGeneration {
			return checkoutOfferReconfirmRequiredError(
				fmt.Errorf(
					"%s: displayed generation %d does not match current %d",
					op,
					input.EvaluationGeneration,
					found.EvaluationGeneration,
				),
			)
		}

		session = found

		return nil
	})
	if err != nil {
		return model.CheckoutSession{}, err
	}

	return session, nil
}
