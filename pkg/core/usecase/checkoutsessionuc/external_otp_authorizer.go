package checkoutsessionuc

import (
	"context"
	"fmt"
	"strconv"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/otpcode"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
)

const (
	// otpNotificationMerchantNameParam is the template placeholder carrying the
	// trusted BNPL merchant display name (never a partner-supplied label).
	otpNotificationMerchantNameParam = "merchant_name"

	// otpNotificationIdempotencyPrefix namespaces the Notification idempotency
	// key of one OTP issuance. The otp use case appends the issue id.
	otpNotificationIdempotencyPrefix = "checkout-otp:"
)

// hostedCheckoutOTPAuthorizer authorizes hosted-checkout OTP issuance under the
// canonical OTP×Saga×FC lock order before otp.Request locks the challenge:
//
//	Saga → Contract → PaymentAttempt → CheckoutSession → OTPChallenge → RL(phone) → RL(merchant)
//
// Open sessions lock CheckoutSession only. Continuation (confirming/processing)
// locks Saga/Contract/PA first, then CheckoutSession, so concurrent Saga steps
// that already hold Saga cannot deadlock against OTP issuance waiting on Saga
// while holding CS.
type hostedCheckoutOTPAuthorizer struct {
	uc                 *UseCase
	token              model.CheckoutSessionToken
	expectedGeneration int
	merchantName       string
	templateID         string
}

func (a *hostedCheckoutOTPAuthorizer) AuthorizeIssuance(
	ctx context.Context,
	tx repo.Tx,
) (otp.AuthorizedIssuance, error) {
	const op = "checkoutsessionuc.hostedCheckoutOTPAuthorizer.AuthorizeIssuance"

	peeked, err := a.uc.peekExternalSession(ctx, tx, a.token)
	if err != nil {
		return otp.AuthorizedIssuance{}, err
	}

	var locked model.CheckoutSession
	now := a.uc.clock.Now().UTC()

	switch peeked.Status {
	case model.OpenCheckoutSessionStatus:
		var expired bool
		locked, expired, err = a.uc.lockExternalOpenForOTPIssuance(ctx, tx, a.token)
		if err != nil {
			return otp.AuthorizedIssuance{}, err
		}

		if expired {
			return otp.AuthorizedIssuance{}, checkoutSessionExpiredError()
		}

		if err := a.uc.assertInitialOTPIssuanceEligible(locked, now, op); err != nil {
			return otp.AuthorizedIssuance{}, err
		}

	case model.ConfirmingCheckoutSessionStatus, model.ProcessingCheckoutSessionStatus:
		// Canonical order: Saga → Contract → PA before CheckoutSession.
		if _, err := a.uc.lockOTPContinuationDepsInTx(ctx, tx, peeked); err != nil {
			return otp.AuthorizedIssuance{}, err
		}

		locked, err = a.uc.lockExternalSession(ctx, tx, a.token)
		if err != nil {
			return otp.AuthorizedIssuance{}, err
		}

		if err := assertContinuationSessionMatchesPeeked(locked, peeked, op); err != nil {
			return otp.AuthorizedIssuance{}, err
		}

		if locked.CustomerID == nil || locked.CustomerID.IsZero() {
			return otp.AuthorizedIssuance{}, checkoutOTPIssuanceNotAllowedError(
				fmt.Errorf("%s: customer binding is required", op),
			)
		}

	default:
		return otp.AuthorizedIssuance{}, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: checkout session status is %q", op, peeked.Status),
		)
	}

	if locked.EvaluationGeneration != a.expectedGeneration {
		return otp.AuthorizedIssuance{}, checkoutOfferReconfirmRequiredError(
			fmt.Errorf(
				"%s: displayed generation %d does not match current %d",
				op,
				a.expectedGeneration,
				locked.EvaluationGeneration,
			),
		)
	}

	phone, err := a.uc.decryptCustomerPhone(locked)
	if err != nil {
		return otp.AuthorizedIssuance{}, err
	}

	phoneFingerprint, err := a.uc.otpHasher.Hash(otpcode.PhoneRateLimitMaterial(phone))
	if err != nil {
		return otp.AuthorizedIssuance{}, fmt.Errorf("%s: hash phone rate-limit key: %w", op, err)
	}

	terms := a.uc.otp.Terms()
	contextVersion := strconv.Itoa(locked.EvaluationGeneration)

	return otp.AuthorizedIssuance{
		Purpose:        model.OTPPurposeHostedCheckoutConfirm,
		SubjectKey:     locked.ID.String(),
		ContextVersion: contextVersion,
		Binding:        a.token.String(),
		Recipient:      phone,
		TemplateID:     a.templateID,
		Variables: map[string]string{
			otpNotificationMerchantNameParam: a.merchantName,
		},
		IdempotencyKeyPrefix: otpNotificationIdempotencyPrefix + a.token.String() + ":",
		CorrelationID:        trace.CorrelationID(ctx),
		RateLimits: []otp.RateLimitBudget{
			{
				Scope:    model.OTPRateLimitScopePhone,
				ScopeKey: phoneFingerprint.String(),
				Window:   terms.PhoneSendWindow,
				MaxSends: terms.MaxSendsPerPhone,
			},
			{
				Scope:    model.OTPRateLimitScopeMerchant,
				ScopeKey: locked.MerchantProfileID.String(),
				Window:   terms.MerchantSendWindow,
				MaxSends: terms.MaxSendsPerMerchant,
			},
		},
		Terminal:        false,
		BusinessBlocked: false,
	}, nil
}
