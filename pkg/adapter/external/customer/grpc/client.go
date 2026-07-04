package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	customerv1 "github.com/parstis/bnpl-backend/gen/proto/customer/v1"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

const (
	defaultTimeout = 5 * time.Second

	circuitResolveCustomer = "external.customer.resolve_customer_by_mobile"

	// correlationMetadataKey is required by Customer Reference Service on every
	// RPC. gRPC metadata keys are lowercase wire names (not HTTP header case).
	correlationMetadataKey = "x-correlation-id"
)

var _ extcustomer.Resolver = (*Client)(nil)

// Client calls the Customer Reference Service over gRPC.
//
// Requests carry customer mobiles, so the client never logs request or
// response payloads and never embeds raw upstream gRPC status messages in
// returned errors (they may contain phone numbers).
type Client struct {
	client  customerv1.CustomerReferenceServiceClient
	timeout time.Duration
	breaker circuitbreaker.CircuitBreaker
}

// New constructs a gRPC Customer Reference Service client using the supplied
// connection.
func New(
	conn grpcclient.ClientConnInterface,
	timeout time.Duration,
	breaker circuitbreaker.CircuitBreaker,
) (*Client, error) {
	const op = "grpc.New"

	if conn == nil {
		return nil, fmt.Errorf("%s: customer grpc connection is required", op)
	}

	return newClient(customerv1.NewCustomerReferenceServiceClient(conn), timeout, breaker)
}

func newClient(
	client customerv1.CustomerReferenceServiceClient,
	timeout time.Duration,
	breaker circuitbreaker.CircuitBreaker,
) (*Client, error) {
	const op = "grpc.newClient"

	if client == nil {
		return nil, fmt.Errorf("%s: customer reference service client is required", op)
	}

	if breaker == nil {
		return nil, fmt.Errorf("%s: circuit breaker is required", op)
	}

	if timeout <= 0 {
		timeout = defaultTimeout
	}

	return &Client{client: client, timeout: timeout, breaker: breaker}, nil
}

// ResolveByMobile resolves the Customer Reference Service customer that owns
// the merchant-asserted customer mobile.
//
// Mapping:
//   - NOT_FOUND → cerr.ResourceNotFound
//   - DEADLINE_EXCEEDED / UNAVAILABLE / circuit open → cerr.ServiceUnavailable
//   - PERMISSION_DENIED → fail closed (not customer unavailable)
//   - RESOURCE_EXHAUSTED → cerr.TooManyRequests when available, else wrapped
//   - CUSTOMER_STATUS_UNSPECIFIED → fail closed as protocol error
func (c *Client) ResolveByMobile(
	ctx context.Context,
	input extcustomer.ResolveByMobileInput,
) (extcustomer.ResolveByMobileOutput, error) {
	const op = "grpc.Client.ResolveByMobile"

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	callCtx, err := withOutboundCorrelationMetadata(callCtx)
	if err != nil {
		return extcustomer.ResolveByMobileOutput{}, fmt.Errorf("%s: %w", op, err)
	}

	request := &customerv1.ResolveCustomerByMobileRequest{
		MobileNumber: input.MobileE164,
	}

	var response *customerv1.ResolveCustomerByMobileResponse

	err = c.breaker.Execute(callCtx, circuitResolveCustomer, func(ctx context.Context) error {
		var callErr error

		response, callErr = c.client.ResolveCustomerByMobile(ctx, request)

		return callErr
	})
	if err != nil {
		return extcustomer.ResolveByMobileOutput{}, mapCustomerDependencyError(op, err)
	}

	if response == nil {
		return extcustomer.ResolveByMobileOutput{}, fmt.Errorf("%s: nil response", op)
	}

	// Wire values are used as returned. Do not TrimSpace — padded customer_id
	// values are protocol violations. google/uuid.Parse accepts surrounding
	// whitespace, so reject it explicitly.
	customerIDRaw := response.GetCustomerId()
	if customerIDRaw != strings.TrimSpace(customerIDRaw) {
		return extcustomer.ResolveByMobileOutput{}, fmt.Errorf("%s: customer id has surrounding whitespace", op)
	}

	externalCustomerID, err := model.ParseExternalCustomerID(customerIDRaw)
	if err != nil {
		return extcustomer.ResolveByMobileOutput{}, fmt.Errorf("%s: parse customer id: %w", op, err)
	}

	if externalCustomerID.IsZero() {
		return extcustomer.ResolveByMobileOutput{}, fmt.Errorf("%s: customer id is zero", op)
	}

	statusValue, err := mapCustomerStatus(response.GetStatus())
	if err != nil {
		return extcustomer.ResolveByMobileOutput{}, fmt.Errorf("%s: %w", op, err)
	}

	return extcustomer.ResolveByMobileOutput{
		ExternalCustomerID: externalCustomerID,
		Status:             statusValue,
	}, nil
}

// withOutboundCorrelationMetadata attaches x-correlation-id as gRPC metadata.
// Customer Reference Service rejects calls that omit it. Callers must place the
// inbound correlation id on ctx; this client never mints one.
func withOutboundCorrelationMetadata(ctx context.Context) (context.Context, error) {
	cid := strings.TrimSpace(trace.CorrelationID(ctx))
	if cid == "" {
		return ctx, fmt.Errorf("correlation id is required")
	}

	return metadata.AppendToOutgoingContext(ctx, correlationMetadataKey, cid), nil
}

func mapCustomerStatus(status customerv1.CustomerStatus) (extcustomer.Status, error) {
	switch status {
	case customerv1.CustomerStatus_CUSTOMER_STATUS_ACTIVE:
		return extcustomer.ActiveStatus, nil

	case customerv1.CustomerStatus_CUSTOMER_STATUS_SUSPENDED:
		return extcustomer.SuspendedStatus, nil

	case customerv1.CustomerStatus_CUSTOMER_STATUS_UNSPECIFIED:
		return "", fmt.Errorf("customer status unspecified")

	default:
		return "", fmt.Errorf("unsupported customer status %v", status)
	}
}

func mapCustomerDependencyError(op string, err error) error {
	if errors.Is(err, circuitbreaker.ErrOpen) {
		return cerr.ServiceUnavailable(fmt.Errorf("%s: customer circuit open", op))
	}

	code := status.Code(err)

	switch code {
	case codes.NotFound:
		return cerr.ResourceNotFound(fmt.Errorf("%s: customer not found (grpc_code=%s)", op, code))

	case codes.DeadlineExceeded, codes.Unavailable:
		return cerr.ServiceUnavailable(fmt.Errorf("%s: customer dependency unavailable (grpc_code=%s)", op, code))

	case codes.PermissionDenied, codes.Unauthenticated:
		return cerr.InternalError(fmt.Errorf("%s: customer dependency authorization failure (grpc_code=%s)", op, code))

	case codes.ResourceExhausted:
		return cerr.NewPublic(
			cerr.CodeTooManyRequests,
			"error.default.too_many_requests",
			fmt.Errorf("%s: customer caller rate limit exceeded (grpc_code=%s)", op, code),
			cerr.Details{},
		)

	case codes.InvalidArgument:
		return cerr.InternalError(fmt.Errorf("%s: customer contract input mismatch (grpc_code=%s)", op, code))

	default:
		// Upstream status.Message() may contain the mobile; never wrap err.
		if _, ok := status.FromError(err); ok {
			return fmt.Errorf("%s: customer resolve failed (grpc_code=%s)", op, code)
		}

		return fmt.Errorf("%s: customer resolve failed", op)
	}
}
