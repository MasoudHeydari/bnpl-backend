package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	notificationv1 "github.com/parstis/bnpl-backend/gen/proto/notification/v1"
	cbnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

type fakeNotificationService struct {
	resp *notificationv1.SubmitNotificationResponse
	err  error
	req  *notificationv1.SubmitNotificationRequest
}

func (f *fakeNotificationService) SubmitNotification(
	_ context.Context,
	in *notificationv1.SubmitNotificationRequest,
	_ ...grpcclient.CallOption,
) (*notificationv1.SubmitNotificationResponse, error) {
	f.req = in

	return f.resp, f.err
}

func newTestClient(t *testing.T, fake *fakeNotificationService) *Client {
	t.Helper()

	client, err := newClient(fake, time.Second, cbnoop.New())
	require.NoError(t, err)

	return client
}

func TestSubmit_MapsAcceptedRequest(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(
		2026,
		8,
		9,
		12,
		5,
		0,
		0,
		time.UTC,
	)
	fake := &fakeNotificationService{
		resp: &notificationv1.SubmitNotificationResponse{
			RequestId: "req-abc",
			Status:    notificationv1.SubmissionStatus_SUBMISSION_STATUS_ACCEPTED,
			Duplicate: false,
		},
	}

	ctx := trace.WithCorrelationID(context.Background(), "corr-1")
	out, err := newTestClient(t, fake).Submit(ctx, extnotification.SubmitInput{
		TemplateID:     "bnpl.external_checkout_otp.v1",
		Recipient:      "+989121234567",
		IdempotencyKey: "checkout-otp:cs_x:oi_y",
		CorrelationID:  trace.CorrelationID(ctx),
		ExpiresAt:      expiresAt,
		Variables: map[string]string{
			"code":          "012345",
			"merchant_name": "Shop",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "req-abc", out.RequestID)
	require.False(t, out.Duplicate)

	require.Equal(t, "bnpl.external_checkout_otp.v1", fake.req.GetTemplateId())
	require.Equal(t, "+989121234567", fake.req.GetRecipient())
	require.Equal(t, "checkout-otp:cs_x:oi_y", fake.req.GetIdempotencyKey())
	require.Equal(t, "corr-1", fake.req.GetCorrelationId())
	require.Equal(t, expiresAt, fake.req.GetExpiresAt().AsTime())
	require.Equal(t, "012345", fake.req.GetVariables()["code"])
	require.Equal(t, "Shop", fake.req.GetVariables()["merchant_name"])
}

func TestSubmit_DuplicateAcceptedIsSuccess(t *testing.T) {
	t.Parallel()

	fake := &fakeNotificationService{
		resp: &notificationv1.SubmitNotificationResponse{
			RequestId: "req-abc",
			Status:    notificationv1.SubmissionStatus_SUBMISSION_STATUS_ACCEPTED,
			Duplicate: true,
		},
	}

	out, err := newTestClient(t, fake).Submit(context.Background(), extnotification.SubmitInput{
		TemplateID:     "bnpl.external_checkout_otp.v1",
		Recipient:      "+989121234567",
		IdempotencyKey: "key",
		ExpiresAt:      time.Now().UTC().Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, out.Duplicate)
}

func TestSubmit_PreservesOpaqueRequestIDWhitespace(t *testing.T) {
	t.Parallel()

	fake := &fakeNotificationService{
		resp: &notificationv1.SubmitNotificationResponse{
			RequestId: " req-1 ",
			Status:    notificationv1.SubmissionStatus_SUBMISSION_STATUS_ACCEPTED,
		},
	}

	out, err := newTestClient(t, fake).Submit(context.Background(), extnotification.SubmitInput{
		TemplateID:     "bnpl.external_checkout_otp.v1",
		Recipient:      "+989121234567",
		IdempotencyKey: "key",
		ExpiresAt:      time.Now().UTC().Add(time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, " req-1 ", out.RequestID)
}

func TestSubmit_UnspecifiedStatusIsProtocolFailure(t *testing.T) {
	t.Parallel()

	fake := &fakeNotificationService{
		resp: &notificationv1.SubmitNotificationResponse{
			RequestId: "req-abc",
			Status:    notificationv1.SubmissionStatus_SUBMISSION_STATUS_UNSPECIFIED,
		},
	}

	_, err := newTestClient(t, fake).Submit(context.Background(), extnotification.SubmitInput{
		TemplateID:     "bnpl.external_checkout_otp.v1",
		Recipient:      "+989121234567",
		IdempotencyKey: "key",
		ExpiresAt:      time.Now().UTC().Add(time.Minute),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "submission status")
}

func TestSubmit_NilResponseFails(t *testing.T) {
	t.Parallel()

	fake := &fakeNotificationService{}
	_, err := newTestClient(t, fake).Submit(context.Background(), extnotification.SubmitInput{
		TemplateID:     "bnpl.external_checkout_otp.v1",
		Recipient:      "+989121234567",
		IdempotencyKey: "key",
		ExpiresAt:      time.Now().UTC().Add(time.Minute),
	})
	require.Error(t, err)
}

func TestSubmit_ErrorMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		code         codes.Code
		message      string
		wantCode     cerr.Code
		wantRejected bool
	}{
		{
			name:         "unavailable",
			code:         codes.Unavailable,
			message:      "recipient +989121234567 code=123456",
			wantCode:     cerr.CodeServiceUnavailable,
			wantRejected: false,
		},
		{
			name:         "deadline",
			code:         codes.DeadlineExceeded,
			message:      "deadline for +989121234567",
			wantCode:     cerr.CodeServiceUnavailable,
			wantRejected: false,
		},
		{
			name:         "already exists",
			code:         codes.AlreadyExists,
			message:      "duplicate key for +989121234567 code=123456",
			wantCode:     cerr.CodeInternalError,
			wantRejected: false,
		},
		{
			name:         "invalid argument",
			code:         codes.InvalidArgument,
			message:      "bad variables code=123456 for +989121234567",
			wantCode:     cerr.CodeInternalError,
			wantRejected: true,
		},
		{
			name:         "permission",
			code:         codes.PermissionDenied,
			message:      "template denied for +989121234567",
			wantCode:     cerr.CodeInternalError,
			wantRejected: true,
		},
		{
			name:         "internal",
			code:         codes.Internal,
			message:      "recipient +989121234567 code=123456",
			wantCode:     "",
			wantRejected: false,
		},
		{
			name:         "undocumented not found",
			code:         codes.NotFound,
			message:      "template missing for +989121234567",
			wantCode:     "",
			wantRejected: false,
		},
		{
			name:         "undocumented failed precondition",
			code:         codes.FailedPrecondition,
			message:      "precondition for +989121234567 code=123456",
			wantCode:     "",
			wantRejected: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeNotificationService{err: status.Error(tt.code, tt.message)}
			_, err := newTestClient(t, fake).Submit(context.Background(), extnotification.SubmitInput{
				TemplateID:     "bnpl.external_checkout_otp.v1",
				Recipient:      "+989121234567",
				IdempotencyKey: "key",
				ExpiresAt:      time.Now().UTC().Add(time.Minute),
				Variables:      map[string]string{"code": "999999"},
			})

			require.Equal(t, tt.wantRejected, extnotification.IsRejected(err))
			require.NotContains(t, err.Error(), "123456")
			require.NotContains(t, err.Error(), "999999")
			require.NotContains(t, err.Error(), "+989121234567")
			require.NotContains(t, err.Error(), tt.message)

			if tt.wantCode == "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), "grpc_code=")
				return
			}

			var coded *cerr.Error
			require.ErrorAs(t, err, &coded)
			require.Equal(t, tt.wantCode, coded.Code)
		})
	}
}
