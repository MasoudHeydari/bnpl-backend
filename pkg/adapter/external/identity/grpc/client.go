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

	identityv1 "github.com/parstis/bnpl-backend/gen/proto/identity/v1"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	extidentity "github.com/parstis/bnpl-backend/pkg/core/external/identity"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	defaultTimeout = 5 * time.Second

	circuitResolveSubject = "external.identity.resolve_subject_by_mobile"
)

var _ extidentity.Resolver = (*Client)(nil)

// Client calls the Identity Service IdentityService over gRPC.
//
// Requests carry customer mobiles, so the client never logs request or
// response payloads and never embeds raw upstream gRPC status messages in
// returned errors (they may contain phone numbers).
type Client struct {
	client  identityv1.IdentityServiceClient
	timeout time.Duration
	breaker circuitbreaker.CircuitBreaker
}

// New constructs a gRPC Identity Service client using the supplied connection.
func New(
	conn grpcclient.ClientConnInterface,
	timeout time.Duration,
	breaker circuitbreaker.CircuitBreaker,
) (*Client, error) {
	const op = "grpc.New"

	if conn == nil {
		return nil, fmt.Errorf("%s: identity grpc connection is required", op)
	}

	return newClient(identityv1.NewIdentityServiceClient(conn), timeout, breaker)
}

func newClient(
	client identityv1.IdentityServiceClient,
	timeout time.Duration,
	breaker circuitbreaker.CircuitBreaker,
) (*Client, error) {
	const op = "grpc.newClient"

	if client == nil {
		return nil, fmt.Errorf("%s: identity service client is required", op)
	}

	if breaker == nil {
		return nil, fmt.Errorf("%s: circuit breaker is required", op)
	}

	if timeout <= 0 {
		timeout = defaultTimeout
	}

	return &Client{client: client, timeout: timeout, breaker: breaker}, nil
}

// Dial opens a gRPC client connection to the Identity Service.
func Dial(address string, insecureTransport bool) (*grpcclient.ClientConn, error) {
	const op = "grpc.Dial"

	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("%s: identity grpc address is required", op)
	}

	if !insecureTransport {
		return nil, fmt.Errorf("%s: secure identity grpc transport is not configured", op)
	}

	conn, err := grpcclient.NewClient(address, grpcclient.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("%s: dial identity grpc service: %w", op, err)
	}

	return conn, nil
}

// ResolveByMobile resolves the Identity Service subject that owns the
// merchant-asserted customer mobile.
//
// Mapping:
//   - NOT_FOUND → cerr.ResourceNotFound
//   - DEADLINE_EXCEEDED / UNAVAILABLE / circuit open → cerr.ServiceUnavailable
//   - PERMISSION_DENIED → fail closed (not customer unavailable)
//   - RESOURCE_EXHAUSTED → cerr.TooManyRequests when available, else wrapped
func (c *Client) ResolveByMobile(
	ctx context.Context,
	input extidentity.ResolveByMobileInput,
) (extidentity.ResolveByMobileOutput, error) {
	const op = "grpc.Client.ResolveByMobile"

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	request := &identityv1.ResolveSubjectByMobileRequest{
		MobileNumber: input.MobileE164,
	}

	var response *identityv1.ResolveSubjectByMobileResponse

	err := c.breaker.Execute(callCtx, circuitResolveSubject, func(ctx context.Context) error {
		var callErr error

		response, callErr = c.client.ResolveSubjectByMobile(ctx, request)

		return callErr
	})
	if err != nil {
		return extidentity.ResolveByMobileOutput{}, mapIdentityDependencyError(op, err)
	}

	if response == nil {
		return extidentity.ResolveByMobileOutput{}, fmt.Errorf("%s: nil response", op)
	}

	// Identity wire values are used as returned. Do not TrimSpace — padded
	// subject_id / subject_type / subject_status are protocol violations.
	// google/uuid.Parse accepts surrounding whitespace, so reject it explicitly.
	subjectIDRaw := response.GetSubjectId()
	if subjectIDRaw != strings.TrimSpace(subjectIDRaw) {
		return extidentity.ResolveByMobileOutput{}, fmt.Errorf("%s: subject id has surrounding whitespace", op)
	}

	profileID, err := model.ParseIdentitySubjectID(subjectIDRaw)
	if err != nil {
		return extidentity.ResolveByMobileOutput{}, fmt.Errorf("%s: parse subject id: %w", op, err)
	}

	if profileID.IsZero() {
		return extidentity.ResolveByMobileOutput{}, fmt.Errorf("%s: subject id is zero", op)
	}

	subjectType, err := extidentity.ParseSubjectType(response.GetSubjectType())
	if err != nil {
		return extidentity.ResolveByMobileOutput{}, fmt.Errorf("%s: %w", op, err)
	}

	subjectStatus, err := extidentity.ParseSubjectStatus(response.GetSubjectStatus())
	if err != nil {
		return extidentity.ResolveByMobileOutput{}, fmt.Errorf("%s: %w", op, err)
	}

	return extidentity.ResolveByMobileOutput{
		IdentitySubjectID: profileID,
		SubjectType:       subjectType,
		SubjectStatus:     subjectStatus,
	}, nil
}

func mapIdentityDependencyError(op string, err error) error {
	if errors.Is(err, circuitbreaker.ErrOpen) {
		return cerr.ServiceUnavailable(fmt.Errorf("%s: identity circuit open", op))
	}

	code := status.Code(err)

	switch code {
	case codes.NotFound:
		return cerr.ResourceNotFound(fmt.Errorf("%s: identity subject not found (grpc_code=%s)", op, code))

	case codes.DeadlineExceeded, codes.Unavailable:
		return cerr.ServiceUnavailable(fmt.Errorf("%s: identity dependency unavailable (grpc_code=%s)", op, code))

	case codes.PermissionDenied, codes.Unauthenticated:
		return cerr.InternalError(fmt.Errorf("%s: identity dependency authorization failure (grpc_code=%s)", op, code))

	case codes.ResourceExhausted:
		return cerr.NewPublic(
			cerr.CodeTooManyRequests,
			"error.default.too_many_requests",
			fmt.Errorf("%s: identity caller rate limit exceeded (grpc_code=%s)", op, code),
			cerr.Details{},
		)

	case codes.InvalidArgument:
		return cerr.InternalError(fmt.Errorf("%s: identity contract input mismatch (grpc_code=%s)", op, code))

	default:
		// Upstream status.Message() may contain the mobile; never wrap err.
		if _, ok := status.FromError(err); ok {
			return fmt.Errorf("%s: identity resolve subject failed (grpc_code=%s)", op, code)
		}

		return fmt.Errorf("%s: identity resolve subject failed", op)
	}
}
