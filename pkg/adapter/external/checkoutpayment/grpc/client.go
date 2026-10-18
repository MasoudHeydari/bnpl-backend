package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	paymentv1 "github.com/parstis/bnpl-backend/gen/proto/checkoutpayment/v1"
	collectionobligationv1 "github.com/parstis/bnpl-backend/gen/proto/collectionobligation/v1"
	commonv1 "github.com/parstis/bnpl-backend/gen/proto/common/v1"
	merchantentitlementv1 "github.com/parstis/bnpl-backend/gen/proto/merchantentitlement/v1"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
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
//
// FIXME(financial-delegation): The Identity-issued financial_delegation
// acquisition contract is not yet available. Customer Checkout Payment
// requires this delegation capability (Authorization: Bearer) for
// party_type=customer. Wire the approved Identity integration here when the
// contract is published. Never synthesize, reuse an unrelated JWT, or bypass
// the required customer authorization.
func (c *Client) CreateCheckoutPayment(
	ctx context.Context,
	input extcheckoutpayment.CreateCheckoutPaymentInput,
) (extcheckoutpayment.CreateCheckoutPaymentResult, error) {
	const op = "grpc.Client.CreateCheckoutPayment"

	if strings.TrimSpace(input.PayerType) == extcheckoutpayment.PartyCustomer {
		// Fail closed until Identity financial_delegation can be attached to
		// the outbound gRPC metadata. Do not call Financial Core without it.
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, fmt.Errorf(
			"%s: %w", op, extcheckoutpayment.ErrFinancialDelegationUnavailable,
		)
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	sourceVersion := input.SourceVersion
	request := &paymentv1.CreateCheckoutPaymentRequest{
		Source: &commonv1.BusinessSourceReference{
			Type:    input.SourceType,
			Id:      input.SourceID,
			Version: sourceVersion,
		},
		Payer: &paymentv1.FinancialParty{
			PartyType: input.PayerType,
			PartyId:   input.PayerID,
		},
		Payee: &paymentv1.FinancialParty{
			PartyType: input.PayeeType,
			PartyId:   input.PayeeID,
		},
		PaymentAmount: &commonv1.Money{
			AmountMinor: input.AmountMinor,
			Currency:    input.Currency,
		},
		ProviderCode: input.ProviderCode,
		ReturnUrls: &commonv1.PaymentReturnUrls{
			SuccessUrl: input.SuccessReturnURL,
			FailureUrl: input.FailureReturnURL,
			CancelUrl:  input.CancelReturnURL,
		},
		Description:    input.Description,
		IdempotencyKey: input.IdempotencyKey,
		Target: &paymentv1.CheckoutPaymentTarget{
			Kind: &paymentv1.CheckoutPaymentTarget_CollectionObligation{
				CollectionObligation: &paymentv1.CollectionObligationCheckoutTarget{
					CollectionObligationId: input.CollectionObligationID,
				},
			},
		},
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
		return extcheckoutpayment.CreateCheckoutPaymentResult{}, mapFinancialCoreDependencyError(
			op, fmt.Errorf("%s: financial core create checkout payment: %w", op, err),
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

	sourceVersion := input.SourceVersion
	request := &paymentv1.GetPaymentStatusRequest{
		Source: &commonv1.BusinessSourceReference{
			Type:    input.SourceType,
			Id:      input.SourceID,
			Version: sourceVersion,
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
		return extcheckoutpayment.GetPaymentStatusResult{}, mapFinancialCoreDependencyError(
			op, fmt.Errorf("%s: financial core get payment status: %w", op, err),
		)
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
func (c *Client) PrepareCollectionObligation(
	ctx context.Context,
	input extcheckoutpayment.PrepareCollectionObligationInput,
) (extcheckoutpayment.PrepareCollectionObligationResult, error) {
	const op = "grpc.Client.PrepareCollectionObligation"
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	application, err := mapCollectionApplication(input)
	if err != nil {
		return extcheckoutpayment.PrepareCollectionObligationResult{}, err
	}

	sourceVersion := input.SourceVersion
	req := &collectionobligationv1.PrepareCollectionObligationRequest{
		Source: &commonv1.BusinessSourceReference{
			Type:    input.SourceType,
			Id:      input.SourceID,
			Version: sourceVersion,
		},
		MerchantId:  input.MerchantID,
		CustomerId:  input.CustomerID,
		Amount:      &commonv1.Money{AmountMinor: input.AmountMinor, Currency: input.Currency},
		Application: application,
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
		return extcheckoutpayment.PrepareCollectionObligationResult{}, mapFinancialCoreDependencyError(
			op, fmt.Errorf("%s: %w", op, err),
		)
	}

	if response == nil || strings.TrimSpace(response.GetCollectionObligationId()) == "" {
		return extcheckoutpayment.PrepareCollectionObligationResult{}, fmt.Errorf("%s: invalid response", op)
	}

	outstanding := response.GetOutstandingAmount()
	return extcheckoutpayment.PrepareCollectionObligationResult{
		CollectionObligationID: response.GetCollectionObligationId(),
		OutstandingAmountMinor: outstanding.GetAmountMinor(),
		Currency:               outstanding.GetCurrency(),
		ReusedIdempotentResult: response.GetReusedIdempotentResult(),
	}, nil
}

// RecordMerchantEntitlement synchronously records Financial Core economic recognition for contract activation.
func (c *Client) RecordMerchantEntitlement(
	ctx context.Context,
	input extcheckoutpayment.RecordMerchantEntitlementInput,
) (extcheckoutpayment.RecordMerchantEntitlementResult, error) {
	const op = "grpc.Client.RecordMerchantEntitlement"
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	sourceVersion := input.SourceVersion
	req := &merchantentitlementv1.RecordMerchantEntitlementRequest{
		Source: &commonv1.BusinessSourceReference{
			Type:    input.SourceType,
			Id:      input.SourceID,
			Version: sourceVersion,
		},
		MerchantId: input.MerchantID,
		CustomerId: input.CustomerID,
		Economics: &merchantentitlementv1.MerchantEntitlementEconomics{
			Currency:                          input.Currency,
			CollectedCashAmountMinor:          input.CollectedCashAmountMinor,
			ReceivableAmountMinor:             input.ReceivableAmountMinor,
			MerchantPayableAmountMinor:        input.MerchantPayableAmountMinor,
			MerchantTransactionFeeAmountMinor: input.MerchantTransactionFeeAmountMinor,
		},
	}
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
		return extcheckoutpayment.RecordMerchantEntitlementResult{}, mapFinancialCoreDependencyError(
			op, fmt.Errorf("%s: %w", op, err),
		)
	}

	if response == nil || strings.TrimSpace(response.GetMerchantEntitlementId()) == "" {
		return extcheckoutpayment.RecordMerchantEntitlementResult{}, fmt.Errorf("%s: invalid response", op)
	}

	return extcheckoutpayment.RecordMerchantEntitlementResult{
		MerchantEntitlementID:  response.GetMerchantEntitlementId(),
		ReusedIdempotentResult: response.GetReusedIdempotentResult(),
	}, nil
}

// mapFinancialCoreDependencyError converts transport failures into core error
// classes. gRPC status codes stay in this adapter.
//
// A deadline or cancellation does not prove Financial Core failed to commit.
// Those errors stay retryable with the original idempotency identity.
func mapFinancialCoreDependencyError(op string, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s: financial core call canceled: %w", op, context.Canceled)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return extcheckoutpayment.NewFailure(
			extcheckoutpayment.FailureClassAmbiguous,
			fmt.Errorf("%s: financial core deadline exceeded: %w", op, context.DeadlineExceeded),
		)
	}

	if errors.Is(err, circuitbreaker.ErrOpen) {
		return cerr.ServiceUnavailable(extcheckoutpayment.NewFailure(
			extcheckoutpayment.FailureClassTransient,
			fmt.Errorf("%s: financial core circuit open", op),
		))
	}

	code := status.Code(err)
	switch code {
	case codes.Unavailable:
		return cerr.ServiceUnavailable(extcheckoutpayment.NewFailure(
			extcheckoutpayment.FailureClassTransient,
			fmt.Errorf("%s: financial core unavailable (grpc_code=%s)", op, code),
		))

	case codes.ResourceExhausted:
		return cerr.ServiceUnavailable(extcheckoutpayment.NewFailure(
			extcheckoutpayment.FailureClassTransient,
			fmt.Errorf("%s: financial core resource exhausted (grpc_code=%s)", op, code),
		))

	case codes.DeadlineExceeded:
		return extcheckoutpayment.NewFailure(
			extcheckoutpayment.FailureClassAmbiguous,
			fmt.Errorf("%s: financial core deadline exceeded (grpc_code=%s): %w", op, code, context.DeadlineExceeded),
		)

	case codes.Canceled:
		return fmt.Errorf("%s: financial core call canceled (grpc_code=%s): %w", op, code, context.Canceled)

	case codes.Aborted:
		return cerr.Conflict(extcheckoutpayment.NewFailure(
			extcheckoutpayment.FailureClassRetryableConcurrency,
			fmt.Errorf("%s: financial core concurrency conflict (grpc_code=%s)", op, code),
		))

	case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange:
		return cerr.InvalidArgument(extcheckoutpayment.NewFailure(
			extcheckoutpayment.FailureClassPermanentInvalidInput,
			fmt.Errorf("%s: financial core rejected immutable input (grpc_code=%s)", op, code),
		))

	case codes.AlreadyExists:
		return cerr.Conflict(extcheckoutpayment.NewFailure(
			extcheckoutpayment.FailureClassPermanentConflict,
			fmt.Errorf("%s: financial core reported a conflicting identity (grpc_code=%s)", op, code),
		))

	case codes.OK:
		return err

	default:
		if code == codes.Unknown && status.Code(err) == codes.Unknown {
			if _, ok := status.FromError(err); !ok {
				return err
			}
		}

		return cerr.InternalError(extcheckoutpayment.NewFailure(
			extcheckoutpayment.FailureClassUnknown,
			fmt.Errorf("%s: financial core call failed (grpc_code=%s)", op, code),
		))
	}
}

func mapCollectionApplication(
	input extcheckoutpayment.PrepareCollectionObligationInput,
) (*collectionobligationv1.CollectionApplication, error) {
	const op = "grpc.mapCollectionApplication"

	switch input.Application {
	case extcheckoutpayment.PreRecognitionCashCollectionApplication:
		return &collectionobligationv1.CollectionApplication{
			Kind: &collectionobligationv1.CollectionApplication_PreRecognitionCash{
				PreRecognitionCash: &collectionobligationv1.PreRecognitionCashCollection{},
			},
		}, nil

	case extcheckoutpayment.ExistingReceivableCollectionApplication:
		entitlementID := strings.TrimSpace(input.MerchantEntitlementID)
		if entitlementID == "" {
			return nil, fmt.Errorf("%s: merchant entitlement id is required for existing receivable collection", op)
		}

		return &collectionobligationv1.CollectionApplication{
			Kind: &collectionobligationv1.CollectionApplication_ExistingReceivable{
				ExistingReceivable: &collectionobligationv1.ExistingReceivableCollection{
					MerchantEntitlementId: entitlementID,
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("%s: unsupported collection application %q", op, input.Application)
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
