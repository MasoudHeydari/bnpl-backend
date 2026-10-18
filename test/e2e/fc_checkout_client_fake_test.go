//go:build e2e

package e2e_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

// fcCallKind identifies which Financial Core checkout client method was invoked.
type fcCallKind string

const (
	fcCallPrepareCollectionObligation fcCallKind = "prepare_collection_obligation"
	fcCallCreateCheckoutPayment         fcCallKind = "create_checkout_payment"
	fcCallGetPaymentStatus              fcCallKind = "get_payment_status"
	fcCallRecordMerchantEntitlement     fcCallKind = "record_merchant_entitlement"
)

type fcCallRecord struct {
	Kind      fcCallKind
	SourceID  string
	At        time.Time
	Concurrent bool
}

type fcEntitlementBehavior struct {
	err           error
	delay         time.Duration
	lostAfterCommit bool
}

// controllableCheckoutPaymentClient is a thread-safe Financial Core test double
// for checkout confirmation E2E. It tracks calls and enforces idempotent
// RecordMerchantEntitlement / PrepareCollectionObligation semantics.
type controllableCheckoutPaymentClient struct {
	mu sync.Mutex

	entitlementBehavior fcEntitlementBehavior
	paymentBehavior     fcEntitlementBehavior

	entitlementsBySource map[string]extcheckoutpayment.RecordMerchantEntitlementResult
	obligationsBySource  map[string]extcheckoutpayment.PrepareCollectionObligationResult
	paymentsByKey        map[string]extcheckoutpayment.CreateCheckoutPaymentResult

	inFlightEntitlement map[string]struct{}

	calls []fcCallRecord
}

func newControllableCheckoutPaymentClient() *controllableCheckoutPaymentClient {
	return &controllableCheckoutPaymentClient{
		entitlementsBySource: make(map[string]extcheckoutpayment.RecordMerchantEntitlementResult),
		obligationsBySource:  make(map[string]extcheckoutpayment.PrepareCollectionObligationResult),
		paymentsByKey:        make(map[string]extcheckoutpayment.CreateCheckoutPaymentResult),
		inFlightEntitlement:  make(map[string]struct{}),
	}
}

func (c *controllableCheckoutPaymentClient) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entitlementBehavior = fcEntitlementBehavior{}
	c.paymentBehavior = fcEntitlementBehavior{}
	c.entitlementsBySource = make(map[string]extcheckoutpayment.RecordMerchantEntitlementResult)
	c.obligationsBySource = make(map[string]extcheckoutpayment.PrepareCollectionObligationResult)
	c.paymentsByKey = make(map[string]extcheckoutpayment.CreateCheckoutPaymentResult)
	c.inFlightEntitlement = make(map[string]struct{})
	c.calls = nil
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
	_ context.Context,
	input extcheckoutpayment.CreateCheckoutPaymentInput,
) (extcheckoutpayment.CreateCheckoutPaymentResult, error) {
	const op = "e2e.controllableCheckoutPaymentClient.CreateCheckoutPayment"

	key := input.IdempotencyKey
	if key == "" {
		key = input.SourceID
	}

	c.mu.Lock()
	behavior := c.paymentBehavior
	if existing, ok := c.paymentsByKey[key]; ok {
		c.mu.Unlock()
		c.recordCall(fcCallCreateCheckoutPayment, input.SourceID, false)
		existing.ReusedIdempotentResult = true

		return existing, nil
	}
	c.mu.Unlock()

	if behavior.delay > 0 {
		time.Sleep(behavior.delay)
	}

	if behavior.err != nil {
		c.recordCall(fcCallCreateCheckoutPayment, input.SourceID, false)
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, behavior.err
	}

	c.recordCall(fcCallCreateCheckoutPayment, input.SourceID, false)

	sourceID := input.SourceID
	if sourceID == "" {
		sourceID = "local"
	}

	expiresAt := input.ExpiresAt.UTC()
	if expiresAt.IsZero() {
		expiresAt = time.Now().UTC().Add(15 * time.Minute)
	}

	result := extcheckoutpayment.CreateCheckoutPaymentResult{
		InvoiceID:        fmt.Sprintf("invoice-%s", sourceID),
		PaymentIntentID:  fmt.Sprintf("intent-%s", sourceID),
		PaymentAttemptID: fmt.Sprintf("attempt-%s", sourceID),
		PaymentSessionID: fmt.Sprintf("session-%s", sourceID),
		Status:           extcheckoutpayment.RequiresActionPaymentStatus,
		AmountMinor:      input.AmountMinor,
		Currency:         input.Currency,
		PaymentLink:      fmt.Sprintf("https://example.test/payments/%s", sourceID),
		ExpiresAt:        expiresAt,
	}

	c.mu.Lock()
	c.paymentsByKey[key] = result
	c.mu.Unlock()

	return result, nil
}

func (c *controllableCheckoutPaymentClient) GetPaymentStatus(
	_ context.Context,
	input extcheckoutpayment.GetPaymentStatusInput,
) (extcheckoutpayment.GetPaymentStatusResult, error) {
	c.recordCall(fcCallGetPaymentStatus, input.SourceID, false)

	return extcheckoutpayment.GetPaymentStatusResult{
		PaymentIntentID: input.PaymentIntentID,
		Status:          extcheckoutpayment.RequiresActionPaymentStatus,
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

	c.mu.Lock()
	if existing, ok := c.entitlementsBySource[input.SourceID]; ok {
		c.mu.Unlock()
		c.recordCall(fcCallRecordMerchantEntitlement, input.SourceID, false)
		existing.ReusedIdempotentResult = true

		return existing, nil
	}

	if _, busy := c.inFlightEntitlement[input.SourceID]; busy {
		c.mu.Unlock()
		c.recordCall(fcCallRecordMerchantEntitlement, input.SourceID, true)
		return extcheckoutpayment.RecordMerchantEntitlementResult{}, fmt.Errorf("%s: concurrent entitlement for %s", op, input.SourceID)
	}

	c.inFlightEntitlement[input.SourceID] = struct{}{}
	behavior := c.entitlementBehavior
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.inFlightEntitlement, input.SourceID)
		c.mu.Unlock()
	}()

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
