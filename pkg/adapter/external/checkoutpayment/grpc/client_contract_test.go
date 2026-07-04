package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	paymentv1 "github.com/parstis/bnpl-backend/gen/proto/checkoutpayment/v1"
	commonv1 "github.com/parstis/bnpl-backend/gen/proto/common/v1"
	cbnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

const bufconnSize = 1024 * 1024

// Note: these bufconn tests verify BNPL local mapping and transport compatibility
// against generated stubs. They do not prove wire compatibility with a separately
// deployed Financial Core binary.

type capturingCheckoutPaymentServer struct {
	paymentv1.UnimplementedCheckoutPaymentServiceServer

	lastCreateReq    *paymentv1.CreateCheckoutPaymentRequest
	lastCreateMD     metadata.MD
	lastCreateMethod string
	createResp       *paymentv1.CreateCheckoutPaymentResponse
	createErr        error

	lastStatusReq    *paymentv1.GetPaymentStatusRequest
	lastStatusMethod string
	statusResp       *paymentv1.GetPaymentStatusResponse
	statusErr        error

	createCalls int
}

func (s *capturingCheckoutPaymentServer) CreateCheckoutPayment(
	ctx context.Context,
	req *paymentv1.CreateCheckoutPaymentRequest,
) (*paymentv1.CreateCheckoutPaymentResponse, error) {
	s.createCalls++
	s.lastCreateReq = req
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		s.lastCreateMD = md.Copy()
	}

	if s.createErr != nil {
		return nil, s.createErr
	}

	return s.createResp, nil
}

func (s *capturingCheckoutPaymentServer) GetPaymentStatus(
	_ context.Context,
	req *paymentv1.GetPaymentStatusRequest,
) (*paymentv1.GetPaymentStatusResponse, error) {
	s.lastStatusReq = req
	if s.statusErr != nil {
		return nil, s.statusErr
	}

	return s.statusResp, nil
}

func startCheckoutPaymentBufconn(
	t *testing.T,
	srv *capturingCheckoutPaymentServer,
) *Client {
	t.Helper()

	listener := bufconn.Listen(bufconnSize)
	server := grpc.NewServer(grpc.UnaryInterceptor(
		func(
			ctx context.Context,
			req any,
			info *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler,
		) (any, error) {
			switch info.FullMethod {
			case paymentv1.CheckoutPaymentService_CreateCheckoutPayment_FullMethodName:
				srv.lastCreateMethod = info.FullMethod

			case paymentv1.CheckoutPaymentService_GetPaymentStatus_FullMethodName:
				srv.lastStatusMethod = info.FullMethod
			}

			return handler(ctx, req)
		},
	))
	paymentv1.RegisterCheckoutPaymentServiceServer(server, srv)

	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return New(conn, time.Second, cbnoop.New())
}

func TestCreateCheckoutPayment_GRPCContract_CustomerPlatformEmptyPayee(t *testing.T) {
	t.Parallel()

	expires := time.Date(2026, 10, 20, 12, 30, 0, 0, time.UTC)
	srv := &capturingCheckoutPaymentServer{
		createResp: &paymentv1.CreateCheckoutPaymentResponse{
			InvoiceId:        "inv-1",
			PaymentIntentId:  "pi-1",
			PaymentAttemptId: "fpa-1",
			PaymentSessionId: "ps-1",
			Status:           paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_REQUIRES_CUSTOMER_ACTION,
			PaymentAmount:    &commonv1.Money{AmountMinor: 1000, Currency: "IRR"},
			PaymentLink:      "https://pay.example/ps-1",
			ExpiresAt:        timestamppb.New(expires),
		},
	}
	client := startCheckoutPaymentBufconn(t, srv)

	customerID := "22222222-2222-4222-8222-222222222222"
	in := validCreateCheckoutPaymentInput()
	in.PayerID = customerID

	got, err := client.CreateCheckoutPayment(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, 1, srv.createCalls)
	require.NotNil(t, srv.lastCreateReq)
	require.Equal(t, "/payment.v1.CheckoutPaymentService/CreateCheckoutPayment", srv.lastCreateMethod)
	require.Equal(t, paymentv1.CheckoutPaymentService_CreateCheckoutPayment_FullMethodName, srv.lastCreateMethod)

	req := srv.lastCreateReq
	require.Equal(t, "customer", req.GetPayer().GetPartyType())
	require.Equal(t, customerID, req.GetPayer().GetPartyId())
	require.Equal(t, "platform", req.GetPayee().GetPartyType())
	require.Empty(t, req.GetPayee().GetPartyId())
	require.Equal(t, "co-1", req.GetTarget().GetCollectionObligation().GetCollectionObligationId())
	require.Nil(t, req.GetTarget().GetInvoice())
	require.Equal(t, int64(1000), req.GetPaymentAmount().GetAmountMinor())
	require.Equal(t, "IRR", req.GetPaymentAmount().GetCurrency())
	require.Equal(t, "bnpl_contract", req.GetSource().GetType())
	require.Equal(t, in.SourceID, req.GetSource().GetId())
	require.Equal(t, "idem-1", req.GetIdempotencyKey())
	require.Equal(t, "provider", req.GetProviderCode())
	require.Equal(t, in.SuccessReturnURL, req.GetReturnUrls().GetSuccessUrl())
	require.Equal(t, in.FailureReturnURL, req.GetReturnUrls().GetFailureUrl())
	require.Equal(t, in.CancelReturnURL, req.GetReturnUrls().GetCancelUrl())
	require.Equal(t, in.Description, req.GetDescription())
	require.True(t, req.GetExpiresAt().AsTime().UTC().Equal(in.ExpiresAt.UTC()))
	require.False(t, req.GetExpiresAt().AsTime().IsZero())

	// No customer financial-delegation Bearer metadata.
	if auth := srv.lastCreateMD.Get("authorization"); len(auth) > 0 {
		for _, v := range auth {
			require.NotContains(t, v, "Bearer")
		}
	}

	require.Equal(t, "pi-1", got.PaymentIntentID)
	require.Equal(t, "ps-1", got.PaymentSessionID)
	require.Equal(t, extcheckoutpayment.RequiresActionPaymentStatus, got.Status)
	require.Equal(t, "https://pay.example/ps-1", got.PaymentLink)
	require.False(t, got.ReusedIdempotentResult)
}

func TestCreateCheckoutPayment_GRPCContract_CanonicalizesPartyTypesOnWire(t *testing.T) {
	t.Parallel()

	expires := time.Date(2026, 10, 20, 12, 30, 0, 0, time.UTC)
	srv := &capturingCheckoutPaymentServer{
		createResp: &paymentv1.CreateCheckoutPaymentResponse{
			InvoiceId:        "inv-1",
			PaymentIntentId:  "pi-1",
			PaymentAttemptId: "fpa-1",
			PaymentSessionId: "ps-1",
			Status:           paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_REQUIRES_CUSTOMER_ACTION,
			PaymentAmount:    &commonv1.Money{AmountMinor: 1000, Currency: "IRR"},
			PaymentLink:      "https://pay.example/ps-1",
			ExpiresAt:        timestamppb.New(expires),
		},
	}
	client := startCheckoutPaymentBufconn(t, srv)

	in := validCreateCheckoutPaymentInput()
	in.PayerType = " CUSTOMER "
	in.PayeeType = " Platform "
	in.PayerID = "AaBbCcDd-1111-4111-8111-111111111111"

	_, err := client.CreateCheckoutPayment(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, "customer", srv.lastCreateReq.GetPayer().GetPartyType())
	require.Equal(t, "platform", srv.lastCreateReq.GetPayee().GetPartyType())
	require.Equal(t, "AaBbCcDd-1111-4111-8111-111111111111", srv.lastCreateReq.GetPayer().GetPartyId())
	require.Empty(t, srv.lastCreateReq.GetPayee().GetPartyId())
}

func TestCreateCheckoutPayment_GRPCContract_InstallmentRepaymentShape(t *testing.T) {
	t.Parallel()

	expires := time.Date(2026, 10, 20, 12, 15, 0, 0, time.UTC)
	srv := &capturingCheckoutPaymentServer{
		createResp: &paymentv1.CreateCheckoutPaymentResponse{
			InvoiceId:        "inv-2",
			PaymentIntentId:  "pi-installment-1",
			PaymentAttemptId: "fpa-2",
			PaymentSessionId: "ps-2",
			Status:           paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_REQUIRES_CUSTOMER_ACTION,
			PaymentAmount:    &commonv1.Money{AmountMinor: 20, Currency: "IRR"},
			PaymentLink:      "https://pay.example/ps-2",
			ExpiresAt:        timestamppb.New(expires),
		},
	}
	client := startCheckoutPaymentBufconn(t, srv)

	attemptID := "33333333-3333-4333-8333-333333333333"
	in := validCreateCheckoutPaymentInput()
	in.SourceType = "bnpl_payment_attempt"
	in.SourceID = attemptID
	in.IdempotencyKey = "idem-installment-1"
	in.AmountMinor = 20
	in.Description = "BNPL installment repayment"
	in.ExpiresAt = expires

	got, err := client.CreateCheckoutPayment(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, "/payment.v1.CheckoutPaymentService/CreateCheckoutPayment", srv.lastCreateMethod)
	require.Equal(t, "bnpl_payment_attempt", srv.lastCreateReq.GetSource().GetType())
	require.Equal(t, attemptID, srv.lastCreateReq.GetSource().GetId())
	require.Equal(t, "customer", srv.lastCreateReq.GetPayer().GetPartyType())
	require.Equal(t, "platform", srv.lastCreateReq.GetPayee().GetPartyType())
	require.Empty(t, srv.lastCreateReq.GetPayee().GetPartyId())
	require.Equal(t, "idem-installment-1", srv.lastCreateReq.GetIdempotencyKey())
	require.True(t, srv.lastCreateReq.GetExpiresAt().AsTime().UTC().Equal(expires))
	require.Equal(t, "pi-installment-1", got.PaymentIntentID)
}

func TestCreateCheckoutPayment_GRPCContract_AmbiguousDeadlineNotAuthoritativeFailure(t *testing.T) {
	t.Parallel()

	srv := &capturingCheckoutPaymentServer{
		createErr: status.Error(codes.DeadlineExceeded, "deadline"),
	}
	client := startCheckoutPaymentBufconn(t, srv)

	_, err := client.CreateCheckoutPayment(context.Background(), validCreateCheckoutPaymentInput())
	require.Error(t, err)

	var classified *extcheckoutpayment.ClassifiedFailure
	require.ErrorAs(t, err, &classified)
	require.Equal(t, extcheckoutpayment.FailureClassAmbiguous, classified.Class)
	require.False(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestCreateCheckoutPayment_GRPCContract_ReusedIdempotentResult(t *testing.T) {
	t.Parallel()

	expires := time.Date(2026, 10, 20, 12, 30, 0, 0, time.UTC)
	srv := &capturingCheckoutPaymentServer{
		createResp: &paymentv1.CreateCheckoutPaymentResponse{
			InvoiceId:              "inv-1",
			PaymentIntentId:        "pi-1",
			PaymentAttemptId:       "fpa-1",
			PaymentSessionId:       "ps-1",
			Status:                 paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_REQUIRES_CUSTOMER_ACTION,
			PaymentAmount:          &commonv1.Money{AmountMinor: 1000, Currency: "IRR"},
			PaymentLink:            "https://pay.example/ps-1",
			ExpiresAt:              timestamppb.New(expires),
			ReusedIdempotentResult: true,
		},
	}
	client := startCheckoutPaymentBufconn(t, srv)

	got, err := client.CreateCheckoutPayment(context.Background(), validCreateCheckoutPaymentInput())
	require.NoError(t, err)
	require.True(t, got.ReusedIdempotentResult)
	require.Empty(t, srv.lastCreateReq.GetPayee().GetPartyId())
}

func TestGetPaymentStatus_GRPCContract_NoDelegation(t *testing.T) {
	t.Parallel()

	srv := &capturingCheckoutPaymentServer{
		statusResp: &paymentv1.GetPaymentStatusResponse{
			PaymentIntentId: "pi-1",
			Status:          paymentv1.CheckoutPaymentStatus_CHECKOUT_PAYMENT_STATUS_PROCESSING,
			PaymentAmount:   &commonv1.Money{AmountMinor: 1000, Currency: "IRR"},
		},
	}
	client := startCheckoutPaymentBufconn(t, srv)

	got, err := client.GetPaymentStatus(context.Background(), extcheckoutpayment.GetPaymentStatusInput{
		SourceType:      "bnpl_contract",
		SourceID:        "contract-1",
		PaymentIntentID: "pi-1",
	})
	require.NoError(t, err)
	require.Equal(t, "pi-1", got.PaymentIntentID)
	require.Equal(t, extcheckoutpayment.ProcessingPaymentStatus, got.Status)
	require.Equal(t, "/payment.v1.CheckoutPaymentService/GetPaymentStatus", srv.lastStatusMethod)
	require.Equal(t, paymentv1.CheckoutPaymentService_GetPaymentStatus_FullMethodName, srv.lastStatusMethod)
	require.Equal(t, "bnpl_contract", srv.lastStatusReq.GetSource().GetType())
	require.Equal(t, "contract-1", srv.lastStatusReq.GetSource().GetId())
	require.Equal(t, "pi-1", srv.lastStatusReq.GetPaymentIntentId())
}
