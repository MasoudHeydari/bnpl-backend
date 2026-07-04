package grpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	collectionobligationv1 "github.com/parstis/bnpl-backend/gen/proto/collectionobligation/v1"
	merchantentitlementv1 "github.com/parstis/bnpl-backend/gen/proto/merchantentitlement/v1"
	paymentv1 "github.com/parstis/bnpl-backend/gen/proto/payment/v1"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

var _ extcheckoutpayment.Client = (*Client)(nil)

// Client calls Financial Core CheckoutPaymentService over gRPC.
type Client struct {
	client                     paymentv1.CheckoutPaymentServiceClient
	collectionObligationClient collectionobligationv1.CollectionObligationServiceClient
	merchantEntitlementClient  merchantentitlementv1.MerchantEntitlementServiceClient
	timeout                    time.Duration
	breaker                    circuitbreaker.CircuitBreaker
}

// New constructs a gRPC checkout payment client using the supplied connection.
func New(
	conn *grpcclient.ClientConn,
	timeout time.Duration,
	breaker circuitbreaker.CircuitBreaker,
) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	return &Client{
		client:                     paymentv1.NewCheckoutPaymentServiceClient(conn),
		collectionObligationClient: collectionobligationv1.NewCollectionObligationServiceClient(conn),
		merchantEntitlementClient:  merchantentitlementv1.NewMerchantEntitlementServiceClient(conn),
		timeout:                    timeout,
		breaker:                    breaker,
	}
}

// Dial opens a gRPC client connection to Financial Core.
func Dial(address string, insecureTransport bool) (*grpcclient.ClientConn, error) {
	const op = "grpc.Dial"

	if address == "" {
		return nil, fmt.Errorf("%s: financial core grpc address is required", op)
	}

	var opts []grpcclient.DialOption
	if insecureTransport {
		opts = append(opts, grpcclient.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		return nil, fmt.Errorf("%s: secure financial core grpc transport is not configured", op)
	}

	conn, err := grpcclient.NewClient(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("%s: dial financial core grpc service: %w", op, err)
	}

	return conn, nil
}

// CreateCheckoutPayment creates a checkout payment through Financial Core.
func (c *Client) CreateCheckoutPayment(
	ctx context.Context,
	input extcheckoutpayment.CreateCheckoutPaymentInput,
) (extcheckoutpayment.CreateCheckoutPaymentResult, error) {
	const op = "grpc.Client.CreateCheckoutPayment"

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	request := &paymentv1.CreateCheckoutPaymentRequest{
		Scope: &paymentv1.FinancialScope{
			TenantId:  input.TenantID,
			ContextId: input.ContextID,
		},
		Source: &paymentv1.FinancialSourceReference{
			SourceService: input.SourceService,
			SourceType:    input.SourceType,
			SourceId:      input.SourceID,
			SourceVersion: input.SourceVersion,
		},
		Payer: &paymentv1.FinancialParty{
			PartyType: input.PayerType,
			PartyId:   input.PayerID,
		},
		Payee: &paymentv1.FinancialParty{
			PartyType: input.PayeeType,
			PartyId:   input.PayeeID,
		},
		PaymentAmount: &paymentv1.Money{
			AmountMinor: input.AmountMinor,
			Currency:    input.Currency,
		},
		ProviderCode:           input.ProviderCode,
		SuccessReturnUrl:       input.SuccessReturnURL,
		FailureReturnUrl:       input.FailureReturnURL,
		CancelReturnUrl:        input.CancelReturnURL,
		Description:            input.Description,
		IdempotencyKey:         input.IdempotencyKey,
		RequestFingerprint:     input.RequestFingerprint,
		IdempotencyRecordId:    input.IdempotencyRecordID,
		CollectionObligationId: input.CollectionObligationID,
	}
	if !input.ExpiresAt.IsZero() {
		request.ExpiresAt = timestamppb.New(input.ExpiresAt.UTC())
	}

	var response *paymentv1.CreateCheckoutPaymentResponse
	err := c.breaker.Execute(
		callCtx,
		"external.financial_core.checkout_payment.create",
		func(ctx context.Context) error {
			var callErr error
			response, callErr = c.client.CreateCheckoutPayment(ctx, request)
			return callErr
		},
	)
	if err != nil {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf(
			"%s: financial core create checkout payment: %w", op, err,
		)
	}

	return mapCreateCheckoutPaymentResponse(response)
}

// GetPaymentStatus returns the current checkout payment status from Financial Core.
func (c *Client) GetPaymentStatus(
	ctx context.Context,
	input extcheckoutpayment.GetPaymentStatusInput,
) (extcheckoutpayment.GetPaymentStatusResult, error) {
	const op = "grpc.Client.GetPaymentStatus"
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	request := &paymentv1.GetPaymentStatusRequest{
		Scope: &paymentv1.FinancialScope{TenantId: input.TenantID, ContextId: input.ContextID},
		Source: &paymentv1.FinancialSourceReference{
			SourceService: input.SourceService, SourceType: input.SourceType, SourceId: input.SourceID, SourceVersion: input.SourceVersion,
		},
		PaymentIntentId: input.PaymentIntentID,
	}
	var response *paymentv1.GetPaymentStatusResponse
	err := c.breaker.Execute(callCtx, "external.financial_core.checkout_payment.status", func(ctx context.Context) error {
		var callErr error
		response, callErr = c.client.GetPaymentStatus(ctx, request)
		return callErr
	})
	if err != nil {
		return extcheckoutpayment.GetPaymentStatusResult{}, fmt.Errorf("%s: financial core get payment status: %w", op, err)
	}

	if response == nil {
		return extcheckoutpayment.GetPaymentStatusResult{}, fmt.Errorf("%s: nil response", op)
	}

	status, err := mapCheckoutPaymentStatus(response.GetStatus())
	if err != nil {
		return extcheckoutpayment.GetPaymentStatusResult{}, err
	}

	result := extcheckoutpayment.GetPaymentStatusResult{PaymentIntentID: response.GetPaymentIntentId(), Status: status}
	if amount := response.GetPaymentAmount(); amount != nil {
		result.AmountMinor = amount.GetAmountMinor()
		result.Currency = amount.GetCurrency()
	}

	if v := response.GetCreatedAt(); v != nil {
		result.CreatedAt = v.AsTime().UTC()
	}

	if v := response.GetExpiresAt(); v != nil {
		result.ExpiresAt = v.AsTime().UTC()
	}

	if v := response.GetCompletedAt(); v != nil {
		result.CompletedAt = v.AsTime().UTC()
	}

	return result, nil
}

// PrepareCollectionObligation prepares the generic pre-recognition collection instrument.
func (c *Client) PrepareCollectionObligation(ctx context.Context, input extcheckoutpayment.PrepareCollectionObligationInput) (extcheckoutpayment.PrepareCollectionObligationResult, error) {
	const op = "grpc.Client.PrepareCollectionObligation"
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	application, err := mapCollectionApplication(input.Application)
	if err != nil {
		return extcheckoutpayment.PrepareCollectionObligationResult{}, err
	}

	req := &collectionobligationv1.PrepareCollectionObligationRequest{
		SourceType:            input.SourceType,
		SourceId:              input.SourceID,
		SourceVersion:         input.SourceVersion,
		MerchantId:            input.MerchantID,
		CustomerSubjectId:     input.CustomerSubjectID,
		Currency:              input.Currency,
		AmountMinor:           input.AmountMinor,
		CollectionApplication: application,
		ReceivableId:          strings.TrimSpace(input.ReceivableID),
	}
	if input.Description != "" {
		v := input.Description
		req.Description = &v
	}

	if !input.DueAt.IsZero() {
		req.DueAt = timestamppb.New(input.DueAt.UTC())
	}

	var response *collectionobligationv1.PrepareCollectionObligationResponse
	err = c.breaker.Execute(callCtx, "external.financial_core.collection_obligation.prepare", func(ctx context.Context) error {
		var callErr error
		response, callErr = c.collectionObligationClient.PrepareCollectionObligation(ctx, req)
		return callErr
	})
	if err != nil {
		return extcheckoutpayment.PrepareCollectionObligationResult{}, fmt.Errorf("%s: %w", op, err)
	}

	if response == nil || strings.TrimSpace(response.GetCollectionObligationId()) == "" {
		return extcheckoutpayment.PrepareCollectionObligationResult{}, fmt.Errorf("%s: invalid response", op)
	}

	return extcheckoutpayment.PrepareCollectionObligationResult{CollectionObligationID: response.GetCollectionObligationId(), OutstandingAmountMinor: response.GetOutstandingAmountMinor(), Currency: response.GetCurrency(), ReusedIdempotentResult: response.GetReusedIdempotentResult()}, nil
}

// RecordMerchantEntitlement synchronously records Financial Core economic recognition for contract activation.
func (c *Client) RecordMerchantEntitlement(ctx context.Context, input extcheckoutpayment.RecordMerchantEntitlementInput) (extcheckoutpayment.RecordMerchantEntitlementResult, error) {
	const op = "grpc.Client.RecordMerchantEntitlement"
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req := &merchantentitlementv1.RecordMerchantEntitlementRequest{SourceType: input.SourceType, SourceId: input.SourceID, SourceVersion: input.SourceVersion, MerchantId: input.MerchantID, CustomerSubjectId: input.CustomerSubjectID, Currency: input.Currency, CollectedCashAmountMinor: input.CollectedCashAmountMinor, ReceivableAmountMinor: input.ReceivableAmountMinor, MerchantPayableAmountMinor: input.MerchantPayableAmountMinor, MerchantTransactionFeeAmountMinor: input.MerchantTransactionFeeAmountMinor}
	if input.CollectedCashPaymentIntentID != "" {
		v := input.CollectedCashPaymentIntentID
		req.CollectedCashPaymentIntentId = &v
	}

	var response *merchantentitlementv1.RecordMerchantEntitlementResponse
	err := c.breaker.Execute(callCtx, "external.financial_core.merchant_entitlement.record", func(ctx context.Context) error {
		var callErr error
		response, callErr = c.merchantEntitlementClient.RecordMerchantEntitlement(ctx, req)
		return callErr
	})
	if err != nil {
		return extcheckoutpayment.RecordMerchantEntitlementResult{}, fmt.Errorf("%s: %w", op, err)
	}

	if response == nil || strings.TrimSpace(response.GetMerchantEntitlementId()) == "" {
		return extcheckoutpayment.RecordMerchantEntitlementResult{}, fmt.Errorf("%s: invalid response", op)
	}

	return extcheckoutpayment.RecordMerchantEntitlementResult{
		MerchantEntitlementID:  response.GetMerchantEntitlementId(),
		ReceivableID:           response.GetReceivableId(),
		ReusedIdempotentResult: response.GetReusedIdempotentResult(),
	}, nil
}

func mapCollectionApplication(application extcheckoutpayment.CollectionApplication) (collectionobligationv1.CollectionApplication, error) {
	const op = "grpc.mapCollectionApplication"

	switch application {
	case extcheckoutpayment.PreRecognitionCashCollectionApplication:
		return collectionobligationv1.CollectionApplication_COLLECTION_APPLICATION_PRE_RECOGNITION_CASH, nil

	case extcheckoutpayment.ExistingReceivableCollectionApplication:
		return collectionobligationv1.CollectionApplication_COLLECTION_APPLICATION_EXISTING_RECEIVABLE, nil

	default:
		return collectionobligationv1.CollectionApplication_COLLECTION_APPLICATION_UNSPECIFIED, fmt.Errorf("%s: unsupported collection application %q", op, application)
	}
}

func mapCreateCheckoutPaymentResponse(
	response *paymentv1.CreateCheckoutPaymentResponse,
) (extcheckoutpayment.CreateCheckoutPaymentResult, error) {
	const op = "grpc.mapCreateCheckoutPaymentResponse"

	if response == nil {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf("%s: financial core returned nil checkout payment response", op)
	}

	status, err := mapCheckoutPaymentStatus(response.GetStatus())
	if err != nil {
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, err
	}

	amount := response.GetPaymentAmount()
	result := extcheckoutpayment.CreateCheckoutPaymentResult{
		InvoiceID:              response.GetInvoiceId(),
		PaymentIntentID:        response.GetPaymentIntentId(),
		PaymentAttemptID:       response.GetPaymentAttemptId(),
		PaymentSessionID:       response.GetPaymentSessionId(),
		Status:                 status,
		AmountMinor:            amount.GetAmountMinor(),
		Currency:               amount.GetCurrency(),
		PaymentLink:            response.GetPaymentLink(),
		ReusedIdempotentResult: response.GetReusedIdempotentResult(),
	}

	if expiresAt := response.GetExpiresAt(); expiresAt != nil {
		result.ExpiresAt = expiresAt.AsTime().UTC()
	}

	return result, nil
}

func mapCheckoutPaymentStatus(
	status paymentv1.CheckoutPaymentStatus,
) (extcheckoutpayment.PaymentStatus, error) {
	const op = "grpc.mapCheckoutPaymentStatus"

	switch status {
	case paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_REQUIRES_CUSTOMER_ACTION:
		return extcheckoutpayment.RequiresActionPaymentStatus, nil

	case paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_PROCESSING:
		return extcheckoutpayment.ProcessingPaymentStatus, nil

	case paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_SUCCEEDED:
		return extcheckoutpayment.SucceededPaymentStatus, nil

	case paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_FAILED:
		return extcheckoutpayment.FailedPaymentStatus, nil

	case paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_CANCELLED:
		return extcheckoutpayment.CanceledPaymentStatus, nil

	case paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_EXPIRED:
		return extcheckoutpayment.ExpiredPaymentStatus, nil

	case paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_UNSPECIFIED:
		return "", fmt.Errorf("%s: financial core checkout payment status is unspecified", op)

	default:
		return "", fmt.Errorf("%s: unsupported financial core checkout payment status: %v", op, status)
	}
}
