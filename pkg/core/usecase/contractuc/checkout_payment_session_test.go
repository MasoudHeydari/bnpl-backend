package contractuc

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

type checkoutExpiryLogRecord struct {
	level string
	msg   string
	args  []any
}

type checkoutExpiryRecordingLogger struct {
	mu      *sync.Mutex
	records *[]checkoutExpiryLogRecord
	attrs   []any
}

func newCheckoutExpiryRecordingLogger() *checkoutExpiryRecordingLogger {
	return &checkoutExpiryRecordingLogger{
		mu:      &sync.Mutex{},
		records: &[]checkoutExpiryLogRecord{},
	}
}

func (l *checkoutExpiryRecordingLogger) With(args ...any) corelog.Logger {
	attrs := append(append([]any{}, l.attrs...), args...)
	return &checkoutExpiryRecordingLogger{mu: l.mu, records: l.records, attrs: attrs}
}

func (l *checkoutExpiryRecordingLogger) Debug(msg string, args ...any) {
	l.append("debug", msg, args...)
}

func (l *checkoutExpiryRecordingLogger) Info(msg string, args ...any) {
	l.append("info", msg, args...)
}

func (l *checkoutExpiryRecordingLogger) Warn(msg string, args ...any) {
	l.append("warn", msg, args...)
}

func (l *checkoutExpiryRecordingLogger) Error(msg string, args ...any) {
	l.append("error", msg, args...)
}

func (l *checkoutExpiryRecordingLogger) append(level, msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	combined := append(append([]any{}, l.attrs...), args...)
	*l.records = append(*l.records, checkoutExpiryLogRecord{level: level, msg: msg, args: combined})
}

func (l *checkoutExpiryRecordingLogger) warnCount(msg string) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	count := 0
	for _, record := range *l.records {
		if record.level == "warn" && record.msg == msg {
			count++
		}
	}

	return count
}

func TestBuildCheckoutPaymentInputFromAttempt_CustomerPlatformEmptyPayee(t *testing.T) {
	t.Parallel()

	contractID := model.ContractID(uuid.New())
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	obligationID := "co-checkout-1"
	success := "https://example.test/success"
	failure := "https://example.test/failure"
	cancel := "https://example.test/cancel"
	expires := mustParseRFC3339(t, "2026-10-20T12:30:00Z")
	provider := "provider-a"

	uc := &UseCase{}
	got, err := uc.buildCheckoutPaymentInputFromAttempt(
		model.Contract{
			ID:                    contractID,
			PayableNowAmountMinor: 50_000,
			Currency:              model.IRRCurrency,
			ExpiresAt:             expires,
		},
		model.PaymentAttempt{
			IdempotencyKey:                   "idem-checkout-1",
			FinancialCollectionObligationID:  &obligationID,
			ProviderCode:                     &provider,
			SuccessReturnURL:                 &success,
			FailureReturnURL:                 &failure,
			CancelReturnURL:                  &cancel,
		},
		externalCustomerID,
	)
	require.NoError(t, err)
	require.Equal(t, "customer", got.PayerType)
	require.Equal(t, externalCustomerID.String(), got.PayerID)
	require.Equal(t, "platform", got.PayeeType)
	require.Empty(t, got.PayeeID)
	require.Equal(t, obligationID, got.CollectionObligationID)
	require.Equal(t, "idem-checkout-1", got.IdempotencyKey)
	require.Equal(t, contractID.String(), got.SourceID)
}

func TestValidateExternalPaymentURL(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateExternalPaymentURL("https://financial.example/pay"))

	for name, raw := range map[string]string{
		"http":       "http://financial.example/pay",
		"javascript": "javascript:alert(1)",
		"relative":   "/pay",
		"userinfo":   "https://user:pass@financial.example/pay",
		"empty":      "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, validateExternalPaymentURL(raw))
		})
	}
}

func TestValidatePaymentBasedCreateContractInput_AllowsMissingTenantOrContext(t *testing.T) {
	t.Parallel()

	generic := CreateContractInput{
		DecisionID:        model.DecisionID(uuid.New()),
		ExternalCustomerID: model.ExternalCustomerID(uuid.New()),
	}
	require.NoError(t, validateCreateContractInput(generic))

	paymentBased := CreateContractInput{
		DecisionID:        model.DecisionID(uuid.New()),
		ExternalCustomerID: model.ExternalCustomerID(uuid.New()),
		TenantID:          model.TenantID{},
		ContextID:         model.ContextID{},
	}
	require.NoError(t, validatePaymentBasedCreateContractInput(paymentBased))
}

func TestValidateCheckoutPaymentResult_RejectsHTTPRedirect(t *testing.T) {
	t.Parallel()

	contractRow := model.Contract{
		PayableNowAmountMinor: 100,
		Currency:              model.Currency("IRR"),
		ExpiresAt:             mustParseRFC3339(t, "2026-07-14T12:30:00Z"),
	}

	result := extcheckoutpayment.CreateCheckoutPaymentResult{
		PaymentIntentID:  "payment-intent-123",
		PaymentSessionID: "payment-session-123",
		Status:           extcheckoutpayment.RequiresActionPaymentStatus,
		AmountMinor:      100,
		Currency:         "IRR",
		PaymentLink:      "http://financial.example/pay",
		ExpiresAt:        mustParseRFC3339(t, "2026-07-14T12:15:00Z"),
	}

	uc := &UseCase{logger: lognoop.New()}
	err := uc.validateCheckoutPaymentResult(result, contractRow, mustParseRFC3339(t, "2026-07-14T12:00:00Z"))
	require.Error(t, err)
}

func mustParseRFC3339(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)

	return parsed
}

func TestValidateCheckoutPaymentResult_ValidatesFinancialCoreExpiryWindow(t *testing.T) {
	t.Parallel()

	now := mustParseRFC3339(t, "2026-07-14T12:00:00Z")
	contractExpiresAt := mustParseRFC3339(t, "2026-07-14T12:30:00Z")
	contractRow := model.Contract{
		PayableNowAmountMinor: 100,
		Currency:              model.IRRCurrency,
		ExpiresAt:             contractExpiresAt,
	}
	base := extcheckoutpayment.CreateCheckoutPaymentResult{
		PaymentIntentID:  "payment-intent-123",
		PaymentSessionID: "payment-session-123",
		Status:           extcheckoutpayment.RequiresActionPaymentStatus,
		AmountMinor:      100,
		Currency:         "IRR",
		PaymentLink:      "https://financial.example/pay",
		ExpiresAt:        now.Add(10 * time.Minute),
	}

	logger := newCheckoutExpiryRecordingLogger()
	uc := &UseCase{logger: logger}

	require.NoError(t, uc.validateCheckoutPaymentResult(base, contractRow, now))
	require.Equal(t, 0, logger.warnCount(extcheckoutpayment.LatePaymentSessionExpiryWarning))

	missing := base
	missing.ExpiresAt = time.Time{}
	require.Error(t, uc.validateCheckoutPaymentResult(missing, contractRow, now))

	expired := base
	expired.ExpiresAt = now
	require.Error(t, uc.validateCheckoutPaymentResult(expired, contractRow, now))

	afterContract := base
	afterContract.ExpiresAt = contractExpiresAt.Add(time.Second)
	require.NoError(t, uc.validateCheckoutPaymentResult(afterContract, contractRow, now))
	require.Equal(t, 1, logger.warnCount(extcheckoutpayment.LatePaymentSessionExpiryWarning))
}
