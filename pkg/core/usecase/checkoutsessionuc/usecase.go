package checkoutsessionuc

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
	"github.com/parstis/bnpl-backend/pkg/core/checkouturl"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	checkoutotppolicy "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutotp"
	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/secretbox"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

// decisionCreator is the slice of decisionuc.UseCase the checkout session
// depends on. It exists so evaluation can be exercised without a full decision
// use case.
type decisionCreator interface {
	Create(ctx context.Context, input decisionuc.CreateInput) (decisionuc.CreateOutput, error)
	RevalidateCheckoutOffer(
		ctx context.Context,
		input decisionuc.RevalidateCheckoutOfferInput,
	) (decisionuc.RevalidateCheckoutOfferOutput, error)
}

// contractCreator is the slice of contractuc.UseCase the checkout session
// depends on. It exists so Confirm can be exercised without a full contract
// use case.
type contractCreator interface {
	PrepareCheckoutContract(ctx context.Context, input contractuc.CreateContractInput) (contractuc.CreateContractOutput, error)
	SubmitPreparedCheckoutFulfillment(ctx context.Context, prepared contractuc.CreateContractOutput) (contractuc.CreateContractOutput, error)
}

// UseCase orchestrates merchant checkout handoffs, customer identity
// verification, offer confirmation, cancellation, expiry, and merchant
// lifecycle events.
//
// Lifecycle events (completed / canceled / expired / unavailable) are written to
// the transactional outbox in the same transaction as the state change; see
// LifecycleOutbox for the delivery contract.
//
// TODO(checkout-session-metrics): decide whether create/start/cancel rates belong
// on BusinessMetrics; decisionuc/contractuc already take that dependency.
type UseCase struct {
	pool                       repo.Pool
	checkoutSession            repo.CheckoutSession
	merchantProfile            repo.MerchantProfile
	merchantBranchProfile      repo.MerchantBranchProfile
	customerProfile            repo.CustomerProfile
	contract                   repo.Contract
	paymentAttempt             repo.PaymentAttempt
	installment                repo.Installment
	installmentCharge          repo.InstallmentCharge
	decisionRepo               repo.Decision
	checkoutDecisionAllocation repo.CheckoutDecisionAllocation
	checkoutPolicies           *checkout.Registry
	decision                   decisionCreator
	contractUC                 contractCreator
	lifecycle                  *LifecycleOutbox
	random                     io.Reader
	webhookSecretBox           secretbox.Box
	customerPhoneBox           secretbox.Box
	returnURLPolicy            returndestination.Policy
	clock                      clock.Clock
	checkoutURLs               checkouturl.Builder
	ttl                        time.Duration
	expiryBatchSize            int
	otpHasher                  fingerprint.Hasher
	customerResolver           customer.Resolver
	notificationSubmitter      notification.Submitter
	// jurisdictions gates NEW checkout origination only. Idempotent replay of
	// an existing CheckoutSession must not consult this capability.
	jurisdictions      domaincapability.OriginationJurisdictions
	phoneCanonicalizer phonenumber.Canonicalizer
	otpPolicy          checkoutotppolicy.Policy
	otpTemplateID      string
}

// New constructs a checkout session use case.
//
// checkoutURLs builds hosted checkout page URLs from a startup-validated public
// origin. ttl bounds how long a newly created open session stays usable.
// expiryBatchSize bounds how many open sessions one expiry sweep processes.
// merchantCheckoutEventsDestination is the outbox destination (topic) of
// merchant-facing lifecycle events. webhookSecretBox and customerPhoneBox
// envelope-encrypt webhook signing secrets and external customer phones at
// rest; they use separate key rings. Public checkout tokens are drawn from
// crypto/rand.
//
// Partner checkout: customerResolver maps the Merchant-asserted phone to a
// Customer Reference Service customer_id during offer preparation (Create when
// mode is on_create, or Start when mode is on_start); otpHasher keys the stored
// OTP verifier; notificationSubmitter delivers the code only from customer
// START after explicit Continue. OTP later authorizes confirmation of that exact
// offer generation. Both customer resolution and notification perform network
// I/O and are never called inside a database transaction. otpPolicy supplies
// immutable security terms (TTL, attempt budget, send cooldown, per-checkout and
// cross-checkout send budgets); otpTemplateID selects the Notification template.
func New(
	pool repo.Pool,
	checkoutSession repo.CheckoutSession,
	merchantProfile repo.MerchantProfile,
	merchantBranchProfile repo.MerchantBranchProfile,
	customerProfile repo.CustomerProfile,
	contract repo.Contract,
	paymentAttempt repo.PaymentAttempt,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	decisionRepo repo.Decision,
	checkoutDecisionAllocation repo.CheckoutDecisionAllocation,
	checkoutPolicies *checkout.Registry,
	decision *decisionuc.UseCase,
	contractUC *contractuc.UseCase,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	merchantCheckoutEventsDestination string,
	webhookSecretBox secretbox.Box,
	customerPhoneBox secretbox.Box,
	returnURLPolicy returndestination.Policy,
	clk clock.Clock,
	checkoutURLs checkouturl.Builder,
	ttl time.Duration,
	expiryBatchSize int,
	otpHasher fingerprint.Hasher,
	customerResolver customer.Resolver,
	notificationSubmitter notification.Submitter,
	jurisdictions domaincapability.OriginationJurisdictions,
	phoneCanonicalizer phonenumber.Canonicalizer,
	otpPolicy checkoutotppolicy.Policy,
	otpTemplateID string,
) (*UseCase, error) {
	const op = "checkoutsessionuc.New"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if checkoutSession == nil {
		return nil, fmt.Errorf("%s: checkout session repository is required", op)
	}

	if merchantProfile == nil {
		return nil, fmt.Errorf("%s: merchant profile repository is required", op)
	}

	if merchantBranchProfile == nil {
		return nil, fmt.Errorf("%s: merchant branch profile repository is required", op)
	}

	if customerProfile == nil {
		return nil, fmt.Errorf("%s: customer profile repository is required", op)
	}

	if contract == nil {
		return nil, fmt.Errorf("%s: contract repository is required", op)
	}

	if paymentAttempt == nil {
		return nil, fmt.Errorf("%s: payment attempt repository is required", op)
	}

	if installment == nil {
		return nil, fmt.Errorf("%s: installment repository is required", op)
	}

	if installmentCharge == nil {
		return nil, fmt.Errorf("%s: installment charge repository is required", op)
	}

	if decisionRepo == nil {
		return nil, fmt.Errorf("%s: decision repository is required", op)
	}

	if checkoutDecisionAllocation == nil {
		return nil, fmt.Errorf("%s: checkout decision allocation repository is required", op)
	}

	if checkoutPolicies == nil {
		return nil, fmt.Errorf("%s: checkout policy registry is required", op)
	}

	if decision == nil {
		return nil, fmt.Errorf("%s: decision use case is required", op)
	}

	if contractUC == nil {
		return nil, fmt.Errorf("%s: contract use case is required", op)
	}

	if outbox == nil {
		return nil, fmt.Errorf("%s: outbox repository is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: identifier generator is required", op)
	}

	if webhookSecretBox == nil {
		return nil, fmt.Errorf("%s: webhook secret box is required", op)
	}

	if customerPhoneBox == nil {
		return nil, fmt.Errorf("%s: customer phone box is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if checkoutURLs.IsZero() {
		return nil, fmt.Errorf("%s: checkout url builder is required", op)
	}

	lifecycle, err := NewLifecycleOutbox(
		checkoutSession,
		merchantProfile,
		outbox,
		idGenerator,
		merchantCheckoutEventsDestination,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: create lifecycle outbox: %w", op, err)
	}

	if ttl <= 0 {
		return nil, fmt.Errorf("%s: positive checkout session ttl is required", op)
	}

	if expiryBatchSize <= 0 {
		return nil, fmt.Errorf("%s: positive checkout session expiry batch size is required", op)
	}

	if otpHasher == nil {
		return nil, fmt.Errorf("%s: otp hasher is required", op)
	}

	if customerResolver == nil {
		return nil, fmt.Errorf("%s: customer resolver is required", op)
	}

	if notificationSubmitter == nil {
		return nil, fmt.Errorf("%s: notification submitter is required", op)
	}

	if jurisdictions == nil {
		return nil, fmt.Errorf("%s: jurisdiction capability is required", op)
	}

	if phoneCanonicalizer == nil {
		return nil, fmt.Errorf("%s: phone canonicalizer is required", op)
	}

	if otpPolicy == nil {
		return nil, fmt.Errorf("%s: otp policy is required", op)
	}

	if err := otpPolicy.Terms().Validate(); err != nil {
		return nil, fmt.Errorf("%s: otp policy terms: %w", op, err)
	}

	if otpTemplateID == "" {
		return nil, fmt.Errorf("%s: otp template id is required", op)
	}

	return &UseCase{
		pool:                       pool,
		checkoutSession:            checkoutSession,
		merchantProfile:            merchantProfile,
		merchantBranchProfile:      merchantBranchProfile,
		customerProfile:            customerProfile,
		contract:                   contract,
		paymentAttempt:             paymentAttempt,
		installment:                installment,
		installmentCharge:          installmentCharge,
		decisionRepo:               decisionRepo,
		checkoutDecisionAllocation: checkoutDecisionAllocation,
		checkoutPolicies:           checkoutPolicies,
		decision:                   decision,
		contractUC:                 contractUC,
		lifecycle:                  lifecycle,
		random:                     rand.Reader,
		webhookSecretBox:           webhookSecretBox,
		customerPhoneBox:           customerPhoneBox,
		returnURLPolicy:            returnURLPolicy,
		clock:                      clk,
		checkoutURLs:               checkoutURLs,
		ttl:                        ttl,
		expiryBatchSize:            expiryBatchSize,
		otpHasher:                  otpHasher,
		customerResolver:           customerResolver,
		notificationSubmitter:      notificationSubmitter,
		jurisdictions:              jurisdictions,
		phoneCanonicalizer:         phoneCanonicalizer,
		otpPolicy:                  otpPolicy,
		otpTemplateID:              otpTemplateID,
	}, nil
}
