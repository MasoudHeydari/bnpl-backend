package checkoutsessionuc

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// createOrReplaySession persists a new checkout session or returns the existing
// one for the same merchant order reference.
//
// customerID is non-nil only for trusted internal creates, where the customer
// identity is known before the browser handoff. It also selects the session
// origin: nil means an external partner session.
//
// customerPhone is the raw merchant-asserted phone for external partner creates
// (customerID nil) and must be empty for internal creates. It is canonicalized
// using MerchantProfile.Jurisdiction before envelope encryption and idempotent
// replay comparison.
func (uc *UseCase) createOrReplaySession(
	ctx context.Context,
	request CheckoutSessionRequest,
	customerID *model.CustomerID,
	customerPhone string,
	authTenantID *model.TenantID,
	authContextID *model.ContextID,
) (model.CheckoutSession, CreateResult, error) {
	var (
		session model.CheckoutSession
		result  CreateResult
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					var txErr error
					session, result, txErr = uc.createOrReplaySessionTx(
						ctx,
						tx,
						request,
						customerID,
						customerPhone,
						authTenantID,
						authContextID,
					)

					return txErr
				},
			)
		},
	)
	if err != nil {
		// A concurrent create for the same merchant order reference loses the
		// unique index race. The winner's row is the authoritative session, so
		// resolve against it instead of surfacing a storage conflict.
		if isPlausibleCreateConflict(err) {
			return uc.resolveSessionAfterConflict(ctx, request, customerPhone, authTenantID, authContextID)
		}

		return model.CheckoutSession{}, "", err
	}

	return session, result, nil
}

func (uc *UseCase) createOrReplaySessionTx(
	ctx context.Context,
	tx repo.Tx,
	request CheckoutSessionRequest,
	customerID *model.CustomerID,
	customerPhone string,
	authTenantID *model.TenantID,
	authContextID *model.ContextID,
) (model.CheckoutSession, CreateResult, error) {
	const op = "checkoutsessionuc.UseCase.createOrReplaySessionTx"

	merchantProfileQ := uc.merchantProfile.Tx(tx)
	merchantBranchProfileQ := uc.merchantBranchProfile.Tx(tx)
	sessionQ := uc.checkoutSession.Tx(tx)

	profile, err := merchantProfileQ.GetByMerchantID(ctx, request.MerchantID)
	if err != nil {
		return model.CheckoutSession{}, "", err
	}

	if !profile.Status.IsActive() {
		return model.CheckoutSession{}, "", merchantNotActiveError(
			fmt.Errorf("%s: merchant profile status is %q", op, profile.Status),
		)
	}

	existing, err := sessionQ.LockByMerchantIdempotencyKey(ctx, profile.ID, request.EffectiveIdempotencyKey())
	if cerr.IsCode(err, cerr.CodeResourceNotFound) {
		// No attempt under this key; an active attempt for the same order
		// (created under a legacy key) is still the current attempt.
		existing, err = sessionQ.LockByMerchantOrderID(ctx, profile.ID, request.MerchantOrderID)
		if err == nil && !existing.Status.IsActiveCheckoutAttempt() {
			// A completed checkout already produced the order's contract. A new
			// idempotency key must not start a second paid attempt for it.
			if existing.Status == model.CompletedCheckoutSessionStatus {
				return model.CheckoutSession{}, "", merchantOrderConflictError(
					fmt.Errorf("%s: merchant order already has a completed checkout", op),
				)
			}

			err = cerr.ResourceNotFound(fmt.Errorf("%s: no active checkout attempt for merchant order", op))
		}
	}

	switch {
	case err == nil:
		// Pure idempotent replay of an already-created session. Do not require
		// origination admission: disabling a jurisdiction must not make prior
		// checkouts unreplayable.
		canonicalCustomerPhone, canErr := uc.canonicalizeCustomerPhoneForCreate(
			customerID,
			profile.Jurisdiction,
			customerPhone,
		)
		if canErr != nil {
			return model.CheckoutSession{}, "", canErr
		}

		if !existing.MatchesCreateRequest(
			request.OnlineStoreID,
			request.MerchantOrderID,
			request.CustomerReference,
			request.ReturnDestination,
			request.MerchantState,
			request.RequestedAmountMinor,
			request.Currency,
		) {
			return model.CheckoutSession{}, "", merchantOrderConflictError(
				fmt.Errorf("%s: merchant order id reused with different checkout terms", op),
			)
		}

		if err := uc.assertReplayCustomerPhone(existing, canonicalCustomerPhone); err != nil {
			return model.CheckoutSession{}, "", err
		}

		if err := assertReplayAuthScope(existing, authTenantID, authContextID); err != nil {
			return model.CheckoutSession{}, "", err
		}

		return existing, CreateResultAlreadyExists, nil

	case !cerr.IsCode(err, cerr.CodeResourceNotFound):
		return model.CheckoutSession{}, "", err
	}

	// New checkout attempt / new origination.
	if err := uc.requireOriginationEnabledJurisdiction(op, profile.Jurisdiction); err != nil {
		return model.CheckoutSession{}, "", err
	}

	canonicalCustomerPhone, err := uc.canonicalizeCustomerPhoneForCreate(
		customerID,
		profile.Jurisdiction,
		customerPhone,
	)
	if err != nil {
		return model.CheckoutSession{}, "", err
	}

	// The storefront mapping is the only merchant-resolution input. Merchants
	// never supply a merchant or branch identifier directly.
	branchProfile, err := merchantBranchProfileQ.LockByOnlineStoreID(ctx, request.OnlineStoreID)
	if err != nil {
		return model.CheckoutSession{}, "", err
	}

	if branchProfile.MerchantID != profile.MerchantID {
		return model.CheckoutSession{}, "", onlineStoreNotOwnedError(
			fmt.Errorf("%s: online store resolves to another merchant", op),
		)
	}

	destination, err := sessionQ.GetReturnDestination(ctx, profile.MerchantID, request.ReturnDestination)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.CheckoutSession{}, "", returnDestinationNotRegisteredError(err)
		}

		return model.CheckoutSession{}, "", err
	}

	publicToken, err := model.GenerateCheckoutSessionToken(uc.random)
	if err != nil {
		return model.CheckoutSession{}, "", fmt.Errorf("%s: generate public token: %w", op, err)
	}

	origin := model.ExternalPartnerCheckoutSessionOrigin
	if customerID != nil {
		origin = model.InternalCheckoutSessionOrigin
	}

	phoneCiphertext, phoneKeyID, err := uc.encryptCustomerPhoneForCreate(origin, canonicalCustomerPhone)
	if err != nil {
		return model.CheckoutSession{}, "", err
	}

	now := uc.clock.Now().UTC()

	created, err := sessionQ.Create(
		ctx,
		profile.ID,
		branchProfile.ID,
		request.OnlineStoreID,
		request.MerchantOrderID,
		request.EffectiveIdempotencyKey(),
		publicToken,
		origin,
		request.CustomerReference,
		phoneCiphertext,
		phoneKeyID,
		authTenantID,
		authContextID,
		customerID,
		request.ReturnDestination,
		destination.ReturnURL,
		request.MerchantState,
		request.RequestedAmountMinor,
		request.Currency,
		model.OpenCheckoutSessionStatus,
		now.Add(uc.ttl),
		now,
		now,
	)
	if err != nil {
		return model.CheckoutSession{}, "", err
	}

	return created, CreateResultCreated, nil
}

func (uc *UseCase) resolveSessionAfterConflict(
	ctx context.Context,
	request CheckoutSessionRequest,
	customerPhone string,
	authTenantID *model.TenantID,
	authContextID *model.ContextID,
) (model.CheckoutSession, CreateResult, error) {
	const op = "checkoutsessionuc.UseCase.resolveSessionAfterConflict"

	var session model.CheckoutSession

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			merchantProfileQ := uc.merchantProfile.Conn(conn)
			sessionQ := uc.checkoutSession.Conn(conn)

			profile, profileErr := merchantProfileQ.GetByMerchantID(ctx, request.MerchantID)
			if profileErr != nil {
				return profileErr
			}

			// Conflict recovery resolves an already-created winner. It is not
			// new origination, so do not require origination admission here.
			canonicalCustomerPhone, canErr := uc.canonicalizeCustomerPhoneForCreate(
				nil,
				profile.Jurisdiction,
				customerPhone,
			)
			if canErr != nil {
				return canErr
			}

			existing, existingErr := sessionQ.GetByMerchantIdempotencyKey(
				ctx, profile.ID, request.EffectiveIdempotencyKey(),
			)
			if cerr.IsCode(existingErr, cerr.CodeResourceNotFound) {
				existing, existingErr = sessionQ.GetByMerchantOrderID(ctx, profile.ID, request.MerchantOrderID)
				if existingErr == nil && !existing.Status.IsActiveCheckoutAttempt() {
					existingErr = cerr.ResourceNotFound(fmt.Errorf("%s: no active checkout attempt for merchant order", op))
				}
			}

			if existingErr != nil {
				if cerr.IsCode(existingErr, cerr.CodeResourceNotFound) {
					return checkoutSessionCreateUnresolvedConflictError()
				}

				return existingErr
			}

			if !existing.MatchesCreateRequest(
				request.OnlineStoreID,
				request.MerchantOrderID,
				request.CustomerReference,
				request.ReturnDestination,
				request.MerchantState,
				request.RequestedAmountMinor,
				request.Currency,
			) {
				return merchantOrderConflictError(
					fmt.Errorf("%s: merchant order id reused with different checkout terms", op),
				)
			}

			if phoneErr := uc.assertReplayCustomerPhone(existing, canonicalCustomerPhone); phoneErr != nil {
				return phoneErr
			}

			if scopeErr := assertReplayAuthScope(existing, authTenantID, authContextID); scopeErr != nil {
				return scopeErr
			}

			session = existing

			return nil
		},
	)
	if err != nil {
		return model.CheckoutSession{}, "", err
	}

	return session, CreateResultAlreadyExists, nil
}

// isPlausibleCreateConflict reports whether err is the lost unique-index race
// on (merchant_profile_id, merchant_order_id). The postgres adapter remaps only
// that constraint to Conflict; other ValidationFailed/Conflict causes must not
// be reinterpreted as a create race.
func isPlausibleCreateConflict(err error) bool {
	return cerr.IsCode(err, cerr.CodeConflict)
}

// hostedCheckoutURL builds the BNPL-hosted page the merchant redirects to. The
// merchant receives no other checkout identifier. The public origin is trusted
// deployment configuration validated at startup — never derived from the
// inbound HTTP request.
func (uc *UseCase) hostedCheckoutURL(token model.CheckoutSessionToken) string {
	return uc.checkoutURLs.Checkout(token)
}

// merchantReturnURL appends the public checkout token and opaque merchant state
// to the pre-registered return URL.
//
// The query is a browser convenience only. Merchants must confirm the outcome
// through the status endpoint, never from these parameters. The internal
// checkout session UUID never appears here.
func merchantReturnURL(session model.CheckoutSession) (string, error) {
	const op = "checkoutsessionuc.merchantReturnURL"

	parsed, err := url.Parse(session.ReturnURL)
	if err != nil {
		return "", cerr.InternalError(fmt.Errorf("%s: parse registered return url: %w", op, err))
	}

	query := parsed.Query()
	query.Set("checkout_session_id", session.PublicToken.String())

	if session.MerchantState != "" {
		query.Set("state", session.MerchantState)
	}

	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

// requireOriginationEnabledJurisdiction fails closed when
// MerchantProfile.Jurisdiction may not originate new BNPL business in this
// deployment. This is a domain capability check, not a customer phone
// validation error, and must not be applied to pure idempotent replay.
func (uc *UseCase) requireOriginationEnabledJurisdiction(
	op string,
	jurisdiction model.Jurisdiction,
) error {
	if err := uc.jurisdictions.RequireOriginationEnabled(jurisdiction); err != nil {
		return cerr.InternalError(fmt.Errorf(
			"%s: merchant jurisdiction origination is not enabled for this deployment: %w",
			op,
			err,
		))
	}

	return nil
}

// canonicalizeCustomerPhoneForCreate applies MerchantProfile.Jurisdiction rules
// to the raw merchant-asserted phone for external creates. Internal creates
// must not carry a phone.
func (uc *UseCase) canonicalizeCustomerPhoneForCreate(
	customerID *model.CustomerID,
	jurisdiction model.Jurisdiction,
	rawCustomerPhone string,
) (string, error) {
	const op = "checkoutsessionuc.UseCase.canonicalizeCustomerPhoneForCreate"

	if customerID != nil {
		if rawCustomerPhone != "" {
			return "", fmt.Errorf("%s: internal checkout must not carry a customer phone", op)
		}

		return "", nil
	}

	canonical, err := uc.phoneCanonicalizer.Canonicalize(jurisdiction, rawCustomerPhone)
	if err != nil {
		return "", mapPhoneCanonicalizeError(op, err)
	}

	return canonical, nil
}

func mapPhoneCanonicalizeError(op string, err error) error {
	switch {
	case errors.Is(err, phonenumber.ErrRequired):
		return customerPhoneRequiredError()

	case errors.Is(err, phonenumber.ErrInvalid):
		return customerPhoneInvalidError()

	case errors.Is(err, phonenumber.ErrUnsupportedJurisdiction):
		// Defense in depth: registry refusal is not a customer phone formatting
		// problem. Prefer RequireOriginationEnabled at the business gate so
		// this path is rarely hit.
		return cerr.InternalError(fmt.Errorf(
			"%s: merchant jurisdiction phone rules are not enabled: %w",
			op,
			err,
		))

	default:
		// Do not wrap the adapter error: a future implementation might embed
		// the raw phone in its message. Keep the cause sanitized.
		return cerr.InternalError(fmt.Errorf("%s: phone canonicalizer failed", op))
	}
}

// encryptCustomerPhoneForCreate seals the canonical external customer phone for
// storage. Internal creates never store a phone.
func (uc *UseCase) encryptCustomerPhoneForCreate(
	origin model.CheckoutSessionOrigin,
	canonicalCustomerPhone string,
) (*string, *string, error) {
	const op = "checkoutsessionuc.UseCase.encryptCustomerPhoneForCreate"

	switch origin {
	case model.InternalCheckoutSessionOrigin:
		if canonicalCustomerPhone != "" {
			return nil, nil, fmt.Errorf("%s: internal checkout must not carry a customer phone", op)
		}

		return nil, nil, nil

	case model.ExternalPartnerCheckoutSessionOrigin:
		if canonicalCustomerPhone == "" {
			return nil, nil, customerPhoneRequiredError()
		}

		ciphertext, keyID, err := uc.customerPhoneBox.Encrypt(canonicalCustomerPhone)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: encrypt customer phone: %w", op, err)
		}

		return &ciphertext, &keyID, nil

	default:
		return nil, nil, fmt.Errorf("%s: unsupported checkout session origin %q", op, origin)
	}
}

// assertReplayCustomerPhone reports an idempotency conflict when the replayed
// canonical phone does not match the phone sealed on the existing session.
func (uc *UseCase) assertReplayCustomerPhone(
	session model.CheckoutSession,
	canonicalCustomerPhone string,
) error {
	const op = "checkoutsessionuc.UseCase.assertReplayCustomerPhone"

	switch session.Origin {
	case model.InternalCheckoutSessionOrigin:
		if canonicalCustomerPhone != "" {
			return merchantOrderConflictError(
				fmt.Errorf("%s: internal checkout replay carried a customer phone", op),
			)
		}

		return nil

	case model.ExternalPartnerCheckoutSessionOrigin:
		stored, err := uc.decryptCustomerPhone(session)
		if err != nil {
			return err
		}

		if stored != canonicalCustomerPhone {
			return merchantOrderConflictError(
				fmt.Errorf("%s: customer phone does not match the stored checkout", op),
			)
		}

		return nil

	default:
		return fmt.Errorf("%s: unsupported checkout session origin %q", op, session.Origin)
	}
}

// decryptCustomerPhone opens the envelope-encrypted phone of an external session.
func (uc *UseCase) decryptCustomerPhone(session model.CheckoutSession) (string, error) {
	const op = "checkoutsessionuc.UseCase.decryptCustomerPhone"

	if session.CustomerPhoneCiphertext == nil || session.CustomerPhoneKeyID == nil {
		return "", cerr.InternalError(fmt.Errorf("%s: external session is missing customer phone", op))
	}

	plaintext, err := uc.customerPhoneBox.Decrypt(*session.CustomerPhoneCiphertext, *session.CustomerPhoneKeyID)
	if err != nil {
		return "", cerr.InternalError(fmt.Errorf("%s: decrypt customer phone: %w", op, err))
	}

	return plaintext, nil
}

// assertReplayAuthScope reports an idempotency conflict when the trusted
// Gateway tenant/context of the replay does not match the immutable scope
// snapshotted on the existing External Partner checkout.
func assertReplayAuthScope(
	session model.CheckoutSession,
	authTenantID *model.TenantID,
	authContextID *model.ContextID,
) error {
	const op = "checkoutsessionuc.assertReplayAuthScope"

	switch session.Origin {
	case model.InternalCheckoutSessionOrigin:
		if authTenantID != nil || authContextID != nil {
			return merchantOrderConflictError(
				fmt.Errorf("%s: internal checkout must not carry partner auth scope", op),
			)
		}

		return nil

	case model.ExternalPartnerCheckoutSessionOrigin:
		if authTenantID == nil || authContextID == nil ||
			session.AuthTenantID == nil || session.AuthContextID == nil ||
			*session.AuthTenantID != *authTenantID ||
			*session.AuthContextID != *authContextID {
			return merchantOrderConflictError(
				fmt.Errorf("%s: idempotent replay under a different auth scope", op),
			)
		}

		return nil

	default:
		return merchantOrderConflictError(
			fmt.Errorf("%s: unsupported checkout session origin %q", op, session.Origin),
		)
	}
}
