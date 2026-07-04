//go:build e2e

package e2e_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

// fcCallKind identifies which Financial Core checkout client method was invoked.
type fcCallKind string

const (
	fcCallPrepareCollectionObligation fcCallKind = "prepare_collection_obligation"
	fcCallCreateCheckoutPayment       fcCallKind = "create_checkout_payment"
	fcCallGetPaymentStatus            fcCallKind = "get_payment_status"
	fcCallRecordMerchantEntitlement   fcCallKind = "record_merchant_entitlement"
)

type fcCallRecord struct {
	Kind       fcCallKind
	SourceID   string
	At         time.Time
	Concurrent bool
}

type fcEntitlementBehavior struct {
	err             error
	delay           time.Duration
	lostAfterCommit bool
}

// controllableCheckoutPaymentClient is a thread-safe Financial Core test double
// for checkout confirmation E2E. It tracks calls and enforces idempotent
// RecordMerchantEntitlement / PrepareCollectionObligation semantics.
type paymentAuthoritativeStatus struct {
	status      extcheckoutpayment.PaymentStatus
	amountMinor int64
	currency    string
	completedAt time.Time
}

// committedCheckoutPayment stores the FC result with the canonical request
// fingerprint used for idempotency conflict detection.
type committedCheckoutPayment struct {
	fingerprint string
	result      extcheckoutpayment.CreateCheckoutPaymentResult
}

type controllableCheckoutPaymentClient struct {
	mu sync.Mutex

	entitlementBehavior fcEntitlementBehavior
	paymentBehavior     fcEntitlementBehavior
	// paymentExpiresAtOffset shifts the FC-returned session expiry relative to
	// the BNPL-requested ExpiresAt (soft contract deviation tests).
	paymentExpiresAtOffset time.Duration

	entitlementsBySource map[string]extcheckoutpayment.RecordMerchantEntitlementResult
	obligationsBySource  map[string]extcheckoutpayment.PrepareCollectionObligationResult
	paymentsByKey        map[string]committedCheckoutPayment
	paymentByIntent      map[string]extcheckoutpayment.CreateCheckoutPaymentResult
	authoritativeStatus  map[string]paymentAuthoritativeStatus

	inFlightEntitlement map[string]struct{}
	inFlightPaymentKey  map[string]struct{}
	paymentGuards       map[string]*sync.Mutex
	entitlementGuards   map[string]*sync.Mutex

	calls []fcCallRecord
}

func newControllableCheckoutPaymentClient() *controllableCheckoutPaymentClient {
	return &controllableCheckoutPaymentClient{
		entitlementsBySource: make(map[string]extcheckoutpayment.RecordMerchantEntitlementResult),
		obligationsBySource:  make(map[string]extcheckoutpayment.PrepareCollectionObligationResult),
		paymentsByKey:        make(map[string]committedCheckoutPayment),
		paymentByIntent:      make(map[string]extcheckoutpayment.CreateCheckoutPaymentResult),
		authoritativeStatus:  make(map[string]paymentAuthoritativeStatus),
		inFlightEntitlement:  make(map[string]struct{}),
		inFlightPaymentKey:   make(map[string]struct{}),
		paymentGuards:        make(map[string]*sync.Mutex),
		entitlementGuards:    make(map[string]*sync.Mutex),
	}
}

func (c *controllableCheckoutPaymentClient) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entitlementBehavior = fcEntitlementBehavior{}
	c.paymentBehavior = fcEntitlementBehavior{}
	c.paymentExpiresAtOffset = 0
	c.entitlementsBySource = make(map[string]extcheckoutpayment.RecordMerchantEntitlementResult)
	c.obligationsBySource = make(map[string]extcheckoutpayment.PrepareCollectionObligationResult)
	c.paymentsByKey = make(map[string]committedCheckoutPayment)
	c.paymentByIntent = make(map[string]extcheckoutpayment.CreateCheckoutPaymentResult)
	c.authoritativeStatus = make(map[string]paymentAuthoritativeStatus)
	c.inFlightEntitlement = make(map[string]struct{})
	c.inFlightPaymentKey = make(map[string]struct{})
	c.paymentGuards = make(map[string]*sync.Mutex)
	c.entitlementGuards = make(map[string]*sync.Mutex)
	c.calls = nil
}

// keyGuard serializes callers that share one Financial Core idempotency identity.
// FC guarantees those calls are concurrency-safe and return one logical result.
func (c *controllableCheckoutPaymentClient) keyGuard(table map[string]*sync.Mutex, key string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()

	guard, ok := table[key]
	if !ok {
		guard = &sync.Mutex{}
		table[key] = guard
	}

	return guard
}

func (c *controllableCheckoutPaymentClient) committedPaymentKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]string, 0, len(c.paymentsByKey))
	for key := range c.paymentsByKey {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func (c *controllableCheckoutPaymentClient) committedPaymentByKey(key string) (extcheckoutpayment.CreateCheckoutPaymentResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.paymentsByKey[key]
	if !ok {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, false
	}

	return entry.result, true
}

func (c *controllableCheckoutPaymentClient) setEntitlementSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entitlementBehavior = fcEntitlementBehavior{}
}

func (c *controllableCheckoutPaymentClient) setEntitlementError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entitlementBehavior.err = err
}

func (c *controllableCheckoutPaymentClient) setEntitlementTimeout() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entitlementBehavior.err = context.DeadlineExceeded
}

func (c *controllableCheckoutPaymentClient) setEntitlementDelay(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entitlementBehavior.delay = d
}

func (c *controllableCheckoutPaymentClient) setEntitlementLostAfterCommit() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entitlementBehavior.lostAfterCommit = true
}

func (c *controllableCheckoutPaymentClient) setPaymentSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.paymentBehavior = fcEntitlementBehavior{}
}

func (c *controllableCheckoutPaymentClient) setPaymentLostAfterCommit() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.paymentBehavior.lostAfterCommit = true
}

func (c *controllableCheckoutPaymentClient) setPaymentExpiresAtOffset(offset time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.paymentExpiresAtOffset = offset
}

func (c *controllableCheckoutPaymentClient) setPaymentError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.paymentBehavior.err = err
}

func (c *controllableCheckoutPaymentClient) setPaymentAuthoritativeSucceeded(intentID string, amountMinor uint64, currency string, completedAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.authoritativeStatus[intentID] = paymentAuthoritativeStatus{
		status:      extcheckoutpayment.SucceededPaymentStatus,
		amountMinor: int64(amountMinor),
		currency:    currency,
		completedAt: completedAt,
	}
}

func (c *controllableCheckoutPaymentClient) setPaymentAuthoritativeFailed(intentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.authoritativeStatus[intentID] = paymentAuthoritativeStatus{
		status: extcheckoutpayment.FailedPaymentStatus,
	}
}

func (c *controllableCheckoutPaymentClient) committedPaymentCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.paymentsByKey)
}

func (c *controllableCheckoutPaymentClient) createCheckoutPaymentCallCount(idempotencyKey string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := 0
	for _, call := range c.calls {
		if call.Kind != fcCallCreateCheckoutPayment {
			continue
		}

		if idempotencyKey != "" && call.SourceID != idempotencyKey {
			continue
		}

		n++
	}

	return n
}

func (c *controllableCheckoutPaymentClient) totalCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.calls)
}

func (c *controllableCheckoutPaymentClient) callCount(kind fcCallKind, sourceID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := 0
	for _, call := range c.calls {
		if call.Kind != kind {
			continue
		}

		if sourceID != "" && call.SourceID != sourceID {
			continue
		}

		n++
	}

	return n
}

func (c *controllableCheckoutPaymentClient) callRecords() []fcCallRecord {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]fcCallRecord, len(c.calls))
	copy(out, c.calls)

	return out
}

func (c *controllableCheckoutPaymentClient) committedEntitlementCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.entitlementsBySource)
}

func (c *controllableCheckoutPaymentClient) entitlementCallCount(sourceID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := 0
	for _, call := range c.calls {
		if call.Kind == fcCallRecordMerchantEntitlement && call.SourceID == sourceID {
			n++
		}
	}

	return n
}

func (c *controllableCheckoutPaymentClient) recordCall(kind fcCallKind, sourceID string, concurrent bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls = append(c.calls, fcCallRecord{
		Kind:       kind,
		SourceID:   sourceID,
		At:         time.Now().UTC(),
		Concurrent: concurrent,
	})
}

var _ extcheckoutpayment.Client = (*controllableCheckoutPaymentClient)(nil)

func (c *controllableCheckoutPaymentClient) PrepareCollectionObligation(
	_ context.Context,
	input extcheckoutpayment.PrepareCollectionObligationInput,
) (extcheckoutpayment.PrepareCollectionObligationResult, error) {
	const op = "e2e.controllableCheckoutPaymentClient.PrepareCollectionObligation"

	if input.AmountMinor <= 0 || input.SourceID == "" {
		return extcheckoutpayment.PrepareCollectionObligationResult{}, fmt.Errorf("%s: invalid input", op)
	}

	c.mu.Lock()
	if existing, ok := c.obligationsBySource[input.SourceID]; ok {
		c.mu.Unlock()
		c.recordCall(fcCallPrepareCollectionObligation, input.SourceID, false)
		existing.ReusedIdempotentResult = true

		return existing, nil
	}

	c.mu.Unlock()

	c.recordCall(fcCallPrepareCollectionObligation, input.SourceID, false)

	result := extcheckoutpayment.PrepareCollectionObligationResult{
		CollectionObligationID: fmt.Sprintf("collection-%s", input.SourceID),
		OutstandingAmountMinor: input.AmountMinor,
		Currency:               input.Currency,
	}

	c.mu.Lock()
	c.obligationsBySource[input.SourceID] = result
	c.mu.Unlock()

	return result, nil
}

func (c *controllableCheckoutPaymentClient) CreateCheckoutPayment(
	ctx context.Context,
	input extcheckoutpayment.CreateCheckoutPaymentInput,
) (extcheckoutpayment.CreateCheckoutPaymentResult, error) {
	const op = "e2e.controllableCheckoutPaymentClient.CreateCheckoutPayment"

	// Mirror gRPC normalizeCreateCheckoutPaymentInput: only party_type is
	// rewritten before validation and fingerprinting.
	input.PayerType = canonicalizeFakePartyType(input.PayerType)
	input.PayeeType = canonicalizeFakePartyType(input.PayeeType)

	// Mirror the FC-approved BNPL purchase party contract so mismatched parties
	// cannot silently succeed in Checkout/Saga E2E.
	if input.PayerType != extcheckoutpayment.PartyCustomer {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf(
			"%s: payer type must be %q (got %q)", op, extcheckoutpayment.PartyCustomer, input.PayerType,
		)
	}

	if strings.TrimSpace(input.PayerID) == "" {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf("%s: payer id is required", op)
	}

	if input.PayeeType != extcheckoutpayment.PartyPlatform {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf(
			"%s: payee type must be %q (got %q)", op, extcheckoutpayment.PartyPlatform, input.PayeeType,
		)
	}

	if input.PayeeID != "" {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf(
			"%s: platform payee id must be empty (got %q)", op, input.PayeeID,
		)
	}

	if strings.TrimSpace(input.CollectionObligationID) == "" {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf(
			"%s: collection obligation target is required", op,
		)
	}

	if input.ExpiresAt.IsZero() {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf(
			"%s: expires_at is required", op,
		)
	}

	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		// Mirror the gRPC adapter: idempotency key is required; do not fall back
		// to SourceID and hide missing keys.
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf(
			"%s: idempotency key is required", op,
		)
	}

	fingerprint := createCheckoutPaymentRequestFingerprint(input)

	guard := c.keyGuard(c.paymentGuards, key)
	guard.Lock()
	defer guard.Unlock()

	c.mu.Lock()
	if existing, ok := c.paymentsByKey[key]; ok {
		c.mu.Unlock()
		c.recordCall(fcCallCreateCheckoutPayment, input.SourceID, false)
		if existing.fingerprint != fingerprint {
			return extcheckoutpayment.CreateCheckoutPaymentResult{}, extcheckoutpayment.NewFailure(
				extcheckoutpayment.FailureClassPermanentConflict,
				fmt.Errorf("%s: idempotency key reused with different financial request content", op),
			)
		}

		replay := existing.result
		replay.ReusedIdempotentResult = true

		return replay, nil
	}

	behavior := c.paymentBehavior
	c.mu.Unlock()

	if behavior.delay > 0 {
		select {
		case <-ctx.Done():
			return extcheckoutpayment.CreateCheckoutPaymentResult{}, ctx.Err()

		case <-time.After(behavior.delay):
		}
	}

	if behavior.err != nil && !behavior.lostAfterCommit {
		c.recordCall(fcCallCreateCheckoutPayment, input.SourceID, false)
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, behavior.err
	}

	c.recordCall(fcCallCreateCheckoutPayment, input.SourceID, false)

	c.mu.Lock()
	expiresOffset := c.paymentExpiresAtOffset
	c.mu.Unlock()

	// Default: echo the requested expiry. Optional offset models soft FC
	// contract deviations (later-than-requested session windows).
	expiresAt := input.ExpiresAt.UTC().Add(expiresOffset)

	result := extcheckoutpayment.CreateCheckoutPaymentResult{
		InvoiceID:        fmt.Sprintf("invoice-%s", key),
		PaymentIntentID:  fmt.Sprintf("intent-%s", key),
		PaymentAttemptID: fmt.Sprintf("fc-attempt-%s", key),
		PaymentSessionID: fmt.Sprintf("session-%s", key),
		Status:           extcheckoutpayment.RequiresActionPaymentStatus,
		AmountMinor:      input.AmountMinor,
		Currency:         input.Currency,
		PaymentLink:      fmt.Sprintf("https://example.test/payments/%s", key),
		ExpiresAt:        expiresAt,
	}

	c.mu.Lock()
	c.paymentsByKey[key] = committedCheckoutPayment{
		fingerprint: fingerprint,
		result:      result,
	}
	c.paymentByIntent[result.PaymentIntentID] = result
	c.mu.Unlock()

	if behavior.lostAfterCommit {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, context.DeadlineExceeded
	}

	if behavior.err != nil {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, behavior.err
	}

	return result, nil
}

// createCheckoutPaymentFingerprintPayload is a strongly typed deterministic
// encoding of CreateCheckoutPayment business fields. JSON field order follows
// the struct definition so delimiter collisions cannot forge equality.
type createCheckoutPaymentFingerprintPayload struct {
	SourceType             string `json:"source_type"`
	SourceID               string `json:"source_id"`
	SourceVersionPresent   bool   `json:"source_version_present"`
	SourceVersion          int64  `json:"source_version"`
	PayerType              string `json:"payer_type"`
	PayerID                string `json:"payer_id"`
	PayeeType              string `json:"payee_type"`
	PayeeID                string `json:"payee_id"`
	AmountMinor            int64  `json:"amount_minor"`
	Currency               string `json:"currency"`
	ProviderCode           string `json:"provider_code"`
	CollectionObligationID string `json:"collection_obligation_id"`
	SuccessReturnURL       string `json:"success_return_url"`
	FailureReturnURL       string `json:"failure_return_url"`
	CancelReturnURL        string `json:"cancel_return_url"`
	Description            string `json:"description"`
	ExpiresAt              string `json:"expires_at"`
}

// canonicalizeFakePartyType mirrors the real gRPC adapter: only party_type is
// rewritten (trim + lowercase) before the request is treated as wire-equivalent.
// Description, URLs, and other string fields stay byte-identical to the input.
func canonicalizeFakePartyType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// createCheckoutPaymentRequestFingerprint is a collision-safe, process-local
// fingerprint of CreateCheckoutPayment business fields for fake FC idempotency.
// It matches real gRPC wire semantics: canonicalize party_type only; do not
// TrimSpace Description, return URLs, or other payload strings.
func createCheckoutPaymentRequestFingerprint(input extcheckoutpayment.CreateCheckoutPaymentInput) string {
	payload := createCheckoutPaymentFingerprintPayload{
		SourceType:             input.SourceType,
		SourceID:               input.SourceID,
		PayerType:              canonicalizeFakePartyType(input.PayerType),
		PayerID:                input.PayerID,
		PayeeType:              canonicalizeFakePartyType(input.PayeeType),
		PayeeID:                input.PayeeID,
		AmountMinor:            input.AmountMinor,
		Currency:               input.Currency,
		ProviderCode:           input.ProviderCode,
		CollectionObligationID: input.CollectionObligationID,
		SuccessReturnURL:       input.SuccessReturnURL,
		FailureReturnURL:       input.FailureReturnURL,
		CancelReturnURL:        input.CancelReturnURL,
		Description:            input.Description,
		ExpiresAt:              input.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
	if input.SourceVersion != nil {
		payload.SourceVersionPresent = true
		payload.SourceVersion = *input.SourceVersion
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		// Marshal of a pure string/int struct cannot fail in practice; keep the
		// fake fail-closed if the encoding surface ever changes.
		panic(fmt.Sprintf("createCheckoutPaymentRequestFingerprint: marshal: %v", err))
	}

	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (c *controllableCheckoutPaymentClient) GetPaymentStatus(
	_ context.Context,
	input extcheckoutpayment.GetPaymentStatusInput,
) (extcheckoutpayment.GetPaymentStatusResult, error) {
	c.recordCall(fcCallGetPaymentStatus, input.SourceID, false)

	intentID := input.PaymentIntentID
	c.mu.Lock()
	if authoritative, ok := c.authoritativeStatus[intentID]; ok {
		c.mu.Unlock()

		return extcheckoutpayment.GetPaymentStatusResult{
			PaymentIntentID: intentID,
			Status:          authoritative.status,
			AmountMinor:     authoritative.amountMinor,
			Currency:        authoritative.currency,
			CompletedAt:     authoritative.completedAt,
		}, nil
	}

	stored, ok := c.paymentByIntent[intentID]
	c.mu.Unlock()
	if !ok {
		return extcheckoutpayment.GetPaymentStatusResult{
			PaymentIntentID: intentID,
			Status:          extcheckoutpayment.RequiresActionPaymentStatus,
		}, nil
	}

	return extcheckoutpayment.GetPaymentStatusResult{
		PaymentIntentID: intentID,
		Status:          stored.Status,
		AmountMinor:     stored.AmountMinor,
		Currency:        stored.Currency,
	}, nil
}

func (c *controllableCheckoutPaymentClient) RecordMerchantEntitlement(
	ctx context.Context,
	input extcheckoutpayment.RecordMerchantEntitlementInput,
) (extcheckoutpayment.RecordMerchantEntitlementResult, error) {
	const op = "e2e.controllableCheckoutPaymentClient.RecordMerchantEntitlement"

	if input.SourceID == "" {
		return extcheckoutpayment.RecordMerchantEntitlementResult{}, fmt.Errorf("%s: source id is required", op)
	}

	guard := c.keyGuard(c.entitlementGuards, input.SourceID)
	guard.Lock()
	defer guard.Unlock()

	c.mu.Lock()
	if existing, ok := c.entitlementsBySource[input.SourceID]; ok {
		c.mu.Unlock()
		c.recordCall(fcCallRecordMerchantEntitlement, input.SourceID, false)
		existing.ReusedIdempotentResult = true

		return existing, nil
	}

	behavior := c.entitlementBehavior
	c.mu.Unlock()

	if behavior.delay > 0 {
		select {
		case <-ctx.Done():
			return extcheckoutpayment.RecordMerchantEntitlementResult{}, ctx.Err()

		case <-time.After(behavior.delay):
		}
	}

	if behavior.err != nil && !behavior.lostAfterCommit {
		c.recordCall(fcCallRecordMerchantEntitlement, input.SourceID, false)
		return extcheckoutpayment.RecordMerchantEntitlementResult{}, behavior.err
	}

	result := extcheckoutpayment.RecordMerchantEntitlementResult{
		MerchantEntitlementID: fmt.Sprintf("entitlement-%s", input.SourceID),
	}

	c.mu.Lock()
	c.entitlementsBySource[input.SourceID] = result
	c.mu.Unlock()

	c.recordCall(fcCallRecordMerchantEntitlement, input.SourceID, false)

	if behavior.lostAfterCommit {
		return extcheckoutpayment.RecordMerchantEntitlementResult{}, context.DeadlineExceeded
	}

	if behavior.err != nil {
		return extcheckoutpayment.RecordMerchantEntitlementResult{}, behavior.err
	}

	return result, nil
}
