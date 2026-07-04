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

	notificationv1 "github.com/parstis/bnpl-backend/gen/proto/notification/v1"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
)

const (
	defaultTimeout = 5 * time.Second

	circuitSubmitNotification = "external.notification.submit_notification"
)

var _ extnotification.Submitter = (*Client)(nil)

// Client calls NotificationSubmission over gRPC.
//
// Requests may carry recipient phones and OTP template variables, so the
// client never logs request or response payloads and never embeds raw
// upstream gRPC status messages in returned errors (they may contain PII or
// OTP secrets).
type Client struct {
	client  notificationv1.NotificationSubmissionClient
	timeout time.Duration
	breaker circuitbreaker.CircuitBreaker
}

// New constructs a gRPC Notification Service client using the supplied connection.
func New(
	conn grpcclient.ClientConnInterface,
	timeout time.Duration,
	breaker circuitbreaker.CircuitBreaker,
) (*Client, error) {
	const op = "grpc.New"

	if conn == nil {
		return nil, fmt.Errorf("%s: notification grpc connection is required", op)
	}

	return newClient(notificationv1.NewNotificationSubmissionClient(conn), timeout, breaker)
}

func newClient(
	client notificationv1.NotificationSubmissionClient,
	timeout time.Duration,
	breaker circuitbreaker.CircuitBreaker,
) (*Client, error) {
	const op = "grpc.newClient"

	if client == nil {
		return nil, fmt.Errorf("%s: notification submission client is required", op)
	}

	if breaker == nil {
		return nil, fmt.Errorf("%s: circuit breaker is required", op)
	}

	if timeout <= 0 {
		timeout = defaultTimeout
	}

	return &Client{client: client, timeout: timeout, breaker: breaker}, nil
}

// Dial opens a gRPC client connection to the Notification Service.
func Dial(address string, insecureTransport bool) (*grpcclient.ClientConn, error) {
	const op = "grpc.Dial"

	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("%s: notification grpc address is required", op)
	}

	if !insecureTransport {
		return nil, fmt.Errorf("%s: secure notification grpc transport is not configured", op)
	}

	conn, err := grpcclient.NewClient(address, grpcclient.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("%s: dial notification grpc service: %w", op, err)
	}

	return conn, nil
}

// Submit durably submits one notification. OK + ACCEPTED means Notification
// stored the request, including duplicate=true idempotent replays.
//
// ALREADY_EXISTS is an idempotency invariant violation (same key, different
// payload) and is never treated as normal replay or a replaceable rejection.
func (c *Client) Submit(
	ctx context.Context,
	input extnotification.SubmitInput,
) (extnotification.SubmitOutput, error) {
	const op = "grpc.Client.Submit"

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	request := &notificationv1.SubmitNotificationRequest{
		TemplateId:     input.TemplateID,
		Recipient:      input.Recipient,
		Variables:      input.Variables,
		IdempotencyKey: input.IdempotencyKey,
		CorrelationId:  input.CorrelationID,
		ExpiresAt:      timestamppb.New(input.ExpiresAt.UTC()),
	}

	var response *notificationv1.SubmitNotificationResponse

	err := c.breaker.Execute(callCtx, circuitSubmitNotification, func(ctx context.Context) error {
		var callErr error

		response, callErr = c.client.SubmitNotification(ctx, request)

		return callErr
	})
	if err != nil {
		mapped := mapNotificationDependencyError(op, err)
		if isDefiniteNotificationRejection(err) {
			return extnotification.SubmitOutput{}, extnotification.Rejected(mapped)
		}

		return extnotification.SubmitOutput{}, mapped
	}

	if response == nil {
		return extnotification.SubmitOutput{}, fmt.Errorf("%s: nil response", op)
	}

	if response.GetStatus() != notificationv1.SubmissionStatus_SUBMISSION_STATUS_ACCEPTED {
		return extnotification.SubmitOutput{}, fmt.Errorf(
			"%s: unexpected submission status %s",
			op,
			response.GetStatus().String(),
		)
	}

	// request_id is an opaque upstream identifier; do not TrimSpace or rewrite.
	requestID := response.GetRequestId()
	if requestID == "" {
		return extnotification.SubmitOutput{}, fmt.Errorf("%s: empty request id", op)
	}

	return extnotification.SubmitOutput{
		RequestID: requestID,
		Duplicate: response.GetDuplicate(),
	}, nil
}

// isDefiniteNotificationRejection reports errors that prove Notification did
// not accept the request. Only documented SubmitNotification rejection codes
// qualify. Undocumented statuses (including FailedPrecondition / NotFound)
// remain ambiguous so checkout keeps the in-flight OTP reservation.
func isDefiniteNotificationRejection(err error) bool {
	switch status.Code(err) {
	case codes.InvalidArgument,
		codes.PermissionDenied,
		codes.Unauthenticated:
		return true

	default:
		return false
	}
}

func mapNotificationDependencyError(op string, err error) error {
	if errors.Is(err, circuitbreaker.ErrOpen) {
		return cerr.ServiceUnavailable(fmt.Errorf("%s: notification circuit open", op))
	}

	code := status.Code(err)

	switch code {
	case codes.DeadlineExceeded, codes.Unavailable:
		return cerr.ServiceUnavailable(fmt.Errorf(
			"%s: notification dependency unavailable (grpc_code=%s)",
			op,
			code,
		))

	case codes.AlreadyExists:
		// Same idempotency key with a different payload — not normal replay.
		return cerr.InternalError(fmt.Errorf("%s: notification idempotency invariant violation", op))

	case codes.PermissionDenied, codes.Unauthenticated:
		return cerr.InternalError(fmt.Errorf(
			"%s: notification dependency authorization failure (grpc_code=%s)",
			op,
			code,
		))

	case codes.ResourceExhausted:
		return cerr.NewPublic(
			cerr.CodeTooManyRequests,
			"error.default.too_many_requests",
			fmt.Errorf("%s: notification caller rate limit exceeded (grpc_code=%s)", op, code),
			cerr.Details{},
		)

	case codes.InvalidArgument:
		return cerr.InternalError(fmt.Errorf(
			"%s: notification contract input rejected (grpc_code=%s)",
			op,
			code,
		))

	default:
		// Upstream status.Message() may contain recipient/OTP; never wrap err.
		if _, ok := status.FromError(err); ok {
			return fmt.Errorf("%s: notification submit failed (grpc_code=%s)", op, code)
		}

		return fmt.Errorf("%s: notification submit failed", op)
	}
}
