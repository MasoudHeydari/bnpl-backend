package grpc

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

func TestMapFinancialCoreDependencyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantClass extcheckoutpayment.FailureClass
		wantCode  cerr.Code
		wantIs    error
	}{
		{
			name:      "unavailable is transient bad gateway",
			err:       status.Error(codes.Unavailable, "socket closed"),
			wantClass: extcheckoutpayment.FailureClassTransient,
			wantCode:  cerr.CodeExternalServiceUnavailable,
		},
		{
			name:      "resource exhausted is transient",
			err:       status.Error(codes.ResourceExhausted, "quota"),
			wantClass: extcheckoutpayment.FailureClassTransient,
			wantCode:  cerr.CodeServiceUnavailable,
		},
		{
			name:      "deadline exceeded is ambiguous",
			err:       status.Error(codes.DeadlineExceeded, "timeout"),
			wantClass: extcheckoutpayment.FailureClassAmbiguous,
			wantIs:    context.DeadlineExceeded,
		},
		{
			name:      "context deadline is ambiguous",
			err:       context.DeadlineExceeded,
			wantClass: extcheckoutpayment.FailureClassAmbiguous,
			wantIs:    context.DeadlineExceeded,
		},
		{
			name:      "grpc canceled is ambiguous not local cancellation",
			err:       status.Error(codes.Canceled, "canceled"),
			wantClass: extcheckoutpayment.FailureClassAmbiguous,
		},
		{
			name:   "context canceled preserves cancellation",
			err:    context.Canceled,
			wantIs: context.Canceled,
		},
		{
			name:      "aborted is a retryable concurrency conflict",
			err:       status.Error(codes.Aborted, "race"),
			wantClass: extcheckoutpayment.FailureClassRetryableConcurrency,
			wantCode:  cerr.CodeConflict,
		},
		{
			name:      "invalid argument is permanent invalid input",
			err:       status.Error(codes.InvalidArgument, "bad amount"),
			wantClass: extcheckoutpayment.FailureClassPermanentInvalidInput,
			wantCode:  cerr.CodeInvalidArgument,
		},
		{
			name:      "failed precondition is distinct from invalid argument",
			err:       status.Error(codes.FailedPrecondition, "scope missing"),
			wantClass: extcheckoutpayment.FailureClassPermanentFailedPrecondition,
			wantCode:  cerr.CodeConflict,
		},
		{
			name:      "unauthenticated is permanent auth",
			err:       status.Error(codes.Unauthenticated, "token"),
			wantClass: extcheckoutpayment.FailureClassPermanentAuth,
			wantCode:  cerr.CodeUnauthorized,
		},
		{
			name:      "permission denied is permanent auth",
			err:       status.Error(codes.PermissionDenied, "acl"),
			wantClass: extcheckoutpayment.FailureClassPermanentAuth,
			wantCode:  cerr.CodeForbidden,
		},
		{
			name:      "already exists is a permanent conflict",
			err:       status.Error(codes.AlreadyExists, "identity"),
			wantClass: extcheckoutpayment.FailureClassPermanentConflict,
			wantCode:  cerr.CodeConflict,
		},
		{
			name:      "not found keeps external outcome unknown",
			err:       status.Error(codes.NotFound, "missing"),
			wantClass: extcheckoutpayment.FailureClassNotFound,
			wantCode:  cerr.CodeResourceNotFound,
		},
		{
			name:      "internal grpc code is ambiguous",
			err:       status.Error(codes.Internal, "bug"),
			wantClass: extcheckoutpayment.FailureClassAmbiguous,
			wantCode:  cerr.CodeInternalError,
		},
		{
			name:      "unknown grpc code is ambiguous",
			err:       status.Error(codes.Unknown, "mystery"),
			wantClass: extcheckoutpayment.FailureClassAmbiguous,
			wantCode:  cerr.CodeInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mapFinancialCoreDependencyError(lognoop.New(), "CreateCheckoutPayment", tt.err)
			require.Error(t, got)

			if st, ok := status.FromError(tt.err); ok {
				if msg := strings.Join(strings.Fields(st.Message()), " "); msg != "" {
					require.Contains(t, got.Error(), "grpc_message="+msg)
				}
			}

			if tt.wantIs != nil {
				require.ErrorIs(t, got, tt.wantIs)
			}

			if tt.wantClass != "" {
				var classified *extcheckoutpayment.ClassifiedFailure
				require.ErrorAs(t, got, &classified)
				require.Equal(t, tt.wantClass, classified.Class)
			}

			if tt.wantCode != "" {
				require.True(t, cerr.IsCode(got, tt.wantCode))
			}
		})
	}
}

func TestMapFinancialCoreDependencyErrorMapsPlainNetworkErrorsToBadGateway(t *testing.T) {
	t.Parallel()

	original := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	got := mapFinancialCoreDependencyError(lognoop.New(), "GetPaymentStatus", original)
	require.True(t, cerr.IsCode(got, cerr.CodeExternalServiceUnavailable))
	require.ErrorIs(t, got, original)

	var classified *extcheckoutpayment.ClassifiedFailure
	require.ErrorAs(t, got, &classified)
	require.Equal(t, extcheckoutpayment.FailureClassTransient, classified.Class)
}

func TestMapFinancialCoreDependencyError_IncludesStructuredDetails(t *testing.T) {
	t.Parallel()

	st := status.New(codes.InvalidArgument, "Field is outside the allowed range.")
	detailed, err := st.WithDetails(
		&errdetails.ErrorInfo{
			Domain: "financial",
			Reason: "financial.collection_obligation.request_invalid",
		},
		&errdetails.BadRequest{
			FieldViolations: []*errdetails.BadRequest_FieldViolation{
				{
					Field:       "source.version",
					Description: "Field is outside the allowed range.",
				},
			},
		},
	)
	require.NoError(t, err)

	got := mapFinancialCoreDependencyError(lognoop.New(), "PrepareCollectionObligation", detailed.Err())
	require.Error(t, got)
	require.True(t, cerr.IsCode(got, cerr.CodeInvalidArgument))

	var classified *extcheckoutpayment.ClassifiedFailure
	require.ErrorAs(t, got, &classified)
	require.Equal(t, extcheckoutpayment.FailureClassPermanentInvalidInput, classified.Class)

	msg := got.Error()
	require.Contains(t, msg, "grpc_message=Field is outside the allowed range.")
	require.Contains(t, msg, "domain=financial")
	require.Contains(t, msg, "reason=financial.collection_obligation.request_invalid")
	require.Contains(t, msg, "field=source.version")
	require.Contains(t, msg, "description=Field is outside the allowed range.")
}

func TestMapFinancialCoreDependencyError_PreservesStatusMessageWithoutDetails(t *testing.T) {
	t.Parallel()

	got := mapFinancialCoreDependencyError(
		lognoop.New(),
		"PrepareCollectionObligation",
		status.Error(codes.FailedPrecondition, "Merchant settlement FinancialScope is not configured for this merchant."),
	)
	require.Error(t, got)
	require.True(t, cerr.IsCode(got, cerr.CodeConflict))

	var classified *extcheckoutpayment.ClassifiedFailure
	require.ErrorAs(t, got, &classified)
	require.Equal(t, extcheckoutpayment.FailureClassPermanentFailedPrecondition, classified.Class)

	require.Contains(t, got.Error(), "grpc_code=FailedPrecondition")
	require.Contains(
		t,
		got.Error(),
		"grpc_message=Merchant settlement FinancialScope is not configured for this merchant.",
	)
	require.NotContains(t, got.Error(), "domain=")
}
