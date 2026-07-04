package checkoutsessionuc

import (
	"strings"
	"testing"
	"time"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/canonicalizer/phonenumber/iran"
	"github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
	"github.com/parstis/bnpl-backend/pkg/core/checkouturl"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/external/identity"
	"github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	checkoutorigin "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutorigin"
	checkoutotppolicy "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutotp"
	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/secretbox"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

type newDeps struct {
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
	decision                   *decisionuc.UseCase
	contractUC                 *contractuc.UseCase
	outbox                     repo.Outbox
	webhookSecretBox           secretbox.Box
	customerPhoneBox           secretbox.Box
	checkoutURLs               checkouturl.Builder
	ttl                        time.Duration
	expiryBatchSize            int
	otpHasher                  fingerprint.Hasher
	identityResolver           identity.Resolver
	notificationSubmitter      notification.Submitter
	jurisdictions              domaincapability.OriginationJurisdictions
	phoneCanonicalizer         phonenumber.Canonicalizer
	otpPolicy                  checkoutotppolicy.Policy
	otpTemplateID              string
}

func mustTestCheckoutURLs(t *testing.T) checkouturl.Builder {
	t.Helper()

	origin, err := checkoutorigin.Development([]string{"bnpl.example"}).Parse("https://bnpl.example")
	require.NoError(t, err)

	return checkouturl.New(origin)
}

func newTestSecretBox(t *testing.T) *mock.SecretBox {
	t.Helper()

	box := mock.NewSecretBox(t)
	box.EXPECT().
		Encrypt(testifymock.Anything).
		RunAndReturn(func(plaintext string) (string, string, error) {
			return "sb1." + plaintext, "test", nil
		}).
		Maybe()
	box.EXPECT().
		Decrypt(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ciphertext string, _ string) (string, error) {
			return strings.TrimPrefix(ciphertext, "sb1."), nil
		}).
		Maybe()

	return box
}

func newTestPhoneCanonicalizer(t *testing.T) *mock.PhoneCanonicalizer {
	t.Helper()

	c := mock.NewPhoneCanonicalizer(t)
	iranCanon := iran.New()
	c.EXPECT().
		Canonicalize(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(jurisdiction model.Jurisdiction, raw string) (string, error) {
			if jurisdiction != model.IRJurisdiction {
				return "", phonenumber.ErrUnsupportedJurisdiction
			}

			return iranCanon.Canonicalize(raw)
		}).
		Maybe()

	return c
}

func newTestDeps(t *testing.T) newDeps {
	t.Helper()

	checkoutPolicies, err := checkoutcatalog.NewRegistry()
	require.NoError(t, err)

	identityResolver := mock.NewIdentityResolver(t)
	identityResolver.EXPECT().
		ResolveByMobile(testifymock.Anything, testifymock.Anything).
		Maybe()

	notificationSubmitter := mock.NewNotificationSubmitter(t)
	notificationSubmitter.EXPECT().
		Submit(testifymock.Anything, testifymock.Anything).
		Maybe()

	jurisdictions, err := domaincapability.NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction})
	require.NoError(t, err)

	return newDeps{
		pool:                       mock.NewPool(t),
		checkoutSession:            mock.NewCheckoutSession(t),
		merchantProfile:            mock.NewMerchantProfile(t),
		merchantBranchProfile:      mock.NewMerchantBranchProfile(t),
		customerProfile:            mock.NewCustomerProfile(t),
		contract:                   mock.NewContract(t),
		paymentAttempt:             mock.NewPaymentAttempt(t),
		installment:                mock.NewInstallment(t),
		installmentCharge:          mock.NewInstallmentCharge(t),
		decisionRepo:               mock.NewDecision(t),
		checkoutDecisionAllocation: mock.NewCheckoutDecisionAllocation(t),
		checkoutPolicies:           checkoutPolicies,
		decision:                   &decisionuc.UseCase{},
		contractUC:                 &contractuc.UseCase{},
		outbox:                     mock.NewOutbox(t),
		webhookSecretBox:           newTestSecretBox(t),
		customerPhoneBox:           newTestSecretBox(t),
		checkoutURLs:               mustTestCheckoutURLs(t),
		ttl:                        30 * time.Minute,
		expiryBatchSize:            100,
		otpHasher:                  newTestOTPHasher(t),
		identityResolver:           identityResolver,
		notificationSubmitter:      notificationSubmitter,
		jurisdictions:              jurisdictions,
		phoneCanonicalizer:         newTestPhoneCanonicalizer(t),
		otpPolicy:                  checkoutotppolicy.V1(),
		otpTemplateID:              testOTPTemplateID,
	}
}

func (d newDeps) newUseCase(t *testing.T) (*UseCase, error) {
	t.Helper()

	return New(
		d.pool,
		d.checkoutSession,
		d.merchantProfile,
		d.merchantBranchProfile,
		d.customerProfile,
		d.contract,
		d.paymentAttempt,
		d.installment,
		d.installmentCharge,
		d.decisionRepo,
		d.checkoutDecisionAllocation,
		d.checkoutPolicies,
		d.decision,
		d.contractUC,
		d.outbox,
		mock.NewIdentifierGenerator(t),
		"bnpl.merchant.checkout.events.v1",
		d.webhookSecretBox,
		d.customerPhoneBox,
		returndestination.Development(),
		mock.NewClock(t),
		d.checkoutURLs,
		d.ttl,
		d.expiryBatchSize,
		d.otpHasher,
		d.identityResolver,
		d.notificationSubmitter,
		d.jurisdictions,
		d.phoneCanonicalizer,
		d.otpPolicy,
		d.otpTemplateID,
	)
}

func TestNew_ConstructsWithValidDependencies(t *testing.T) {
	t.Parallel()

	uc, err := newTestDeps(t).newUseCase(t)
	require.NoError(t, err)
	require.NotNil(t, uc)
}

func TestNew_RequiresCheckoutURLBuilder(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t)
	deps.checkoutURLs = checkouturl.Builder{}

	_, err := deps.newUseCase(t)
	require.Error(t, err)
}

func TestNew_RequiresCheckoutSessionRepository(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t)
	deps.checkoutSession = nil

	_, err := deps.newUseCase(t)
	require.Error(t, err)
}

func TestNew_RequiresMerchantProfileRepository(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t)
	deps.merchantProfile = nil

	_, err := deps.newUseCase(t)
	require.Error(t, err)
}

func TestNew_RequiresMerchantBranchProfileRepository(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t)
	deps.merchantBranchProfile = nil

	_, err := deps.newUseCase(t)
	require.Error(t, err)
}

func TestNew_RequiresCustomerProfileRepository(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t)
	deps.customerProfile = nil

	_, err := deps.newUseCase(t)
	require.Error(t, err)
}

func TestNew_RequiresContractUseCase(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t)
	deps.contractUC = nil

	_, err := deps.newUseCase(t)
	require.Error(t, err)
}

func TestNew_RequiresPositiveTTL(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t)
	deps.ttl = 0

	_, err := deps.newUseCase(t)
	require.Error(t, err)
}

func TestNew_RequiresPositiveExpiryBatchSize(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t)
	deps.expiryBatchSize = 0

	_, err := deps.newUseCase(t)
	require.Error(t, err)
}

func TestNew_RequiresOTPDependencies(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*newDeps){
		"otp hasher":              func(d *newDeps) { d.otpHasher = nil },
		"identity resolver":       func(d *newDeps) { d.identityResolver = nil },
		"notification submitter":  func(d *newDeps) { d.notificationSubmitter = nil },
		"jurisdiction capability": func(d *newDeps) { d.jurisdictions = nil },
		"phone canonicalizer":     func(d *newDeps) { d.phoneCanonicalizer = nil },
		"otp policy":              func(d *newDeps) { d.otpPolicy = nil },
		"otp template id":         func(d *newDeps) { d.otpTemplateID = "" },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			deps := newTestDeps(t)
			mutate(&deps)

			_, err := deps.newUseCase(t)
			require.Error(t, err)
		})
	}
}

func TestHostedCheckoutURL_UsesConfiguredOrigin(t *testing.T) {
	t.Parallel()

	uc, err := newTestDeps(t).newUseCase(t)
	require.NoError(t, err)

	token := model.CheckoutSessionToken(
		"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	)
	require.Equal(
		t,
		"https://bnpl.example/checkout/"+token.String(),
		uc.hostedCheckoutURL(token),
	)
}
