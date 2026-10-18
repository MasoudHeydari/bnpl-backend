package grpc

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
			name:      "unavailable is transient",
			err:       status.Error(codes.Unavailable, "socket closed"),
			wantClass: extcheckoutpayment.FailureClassTransient,
			wantCode:  cerr.CodeServiceUnavailable,
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
			name:   "grpc canceled preserves cancellation",
			err:    status.Error(codes.Canceled, "canceled"),
			wantIs: context.Canceled,
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
			name:      "already exists is a permanent conflict",
			err:       status.Error(codes.AlreadyExists, "identity"),
			wantClass: extcheckoutpayment.FailureClassPermanentConflict,
			wantCode:  cerr.CodeConflict,
		},
		{
			name:      "internal grpc code is unknown",
			err:       status.Error(codes.Internal, "bug"),
			wantClass: extcheckoutpayment.FailureClassUnknown,
			wantCode:  cerr.CodeInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mapFinancialCoreDependencyError("CreateCheckoutPayment", tt.err)
			require.Error(t, got)
			require.NotContains(t, got.Error(), "bad amount")
			require.NotContains(t, got.Error(), "socket closed")

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

func TestMapFinancialCoreDependencyErrorPassesPlainNetworkErrorsThrough(t *testing.T) {
	t.Parallel()

	original := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	got := mapFinancialCoreDependencyError("GetPaymentStatus", original)
	require.ErrorIs(t, got, original)

	var opErr *net.OpError
	require.ErrorAs(t, got, &opErr)
}
