package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	customerv1 "github.com/parstis/bnpl-backend/gen/proto/customer/v1"
	cbnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
)

type fakeCustomerService struct {
	resolveResp *customerv1.ResolveCustomerByMobileResponse
	resolveErr  error
	resolveReq  *customerv1.ResolveCustomerByMobileRequest
}

func (f *fakeCustomerService) ResolveCustomerByMobile(
	_ context.Context,
	in *customerv1.ResolveCustomerByMobileRequest,
	_ ...grpcclient.CallOption,
) (*customerv1.ResolveCustomerByMobileResponse, error) {
	f.resolveReq = in

	return f.resolveResp, f.resolveErr
}

func (f *fakeCustomerService) GetCustomer(
	context.Context,
	*customerv1.GetCustomerRequest,
	...grpcclient.CallOption,
) (*customerv1.GetCustomerResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not used")
}

func newTestClient(t *testing.T, fake *fakeCustomerService) *Client {
	t.Helper()

	client, err := newClient(fake, time.Second, cbnoop.New())
	require.NoError(t, err)

	return client
}

func TestNew_RequiresDependencies(t *testing.T) {
	t.Parallel()

	_, err := New(nil, time.Second, cbnoop.New())
	require.Error(t, err)

	_, err = newClient(&fakeCustomerService{}, time.Second, nil)
	require.Error(t, err)
}

func TestResolveByMobile_MapsActive(t *testing.T) {
	t.Parallel()

	customerID := uuid.New()
	fake := &fakeCustomerService{
		resolveResp: &customerv1.ResolveCustomerByMobileResponse{
			CustomerId: customerID.String(),
			Status:     customerv1.CustomerStatus_CUSTOMER_STATUS_ACTIVE,
		},
	}

	out, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extcustomer.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.NoError(t, err)
	require.Equal(t, customerID, out.ExternalCustomerID.UUID())
	require.Equal(t, extcustomer.ActiveStatus, out.Status)
	require.Equal(t, "+989121234567", fake.resolveReq.GetMobileNumber())
}

func TestResolveByMobile_MapsSuspended(t *testing.T) {
	t.Parallel()

	customerID := uuid.New()
	fake := &fakeCustomerService{
		resolveResp: &customerv1.ResolveCustomerByMobileResponse{
			CustomerId: customerID.String(),
			Status:     customerv1.CustomerStatus_CUSTOMER_STATUS_SUSPENDED,
		},
	}

	out, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extcustomer.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.NoError(t, err)
	require.Equal(t, extcustomer.SuspendedStatus, out.Status)
}

func TestResolveByMobile_UnspecifiedStatusIsProtocolError(t *testing.T) {
	t.Parallel()

	fake := &fakeCustomerService{
		resolveResp: &customerv1.ResolveCustomerByMobileResponse{
			CustomerId: uuid.New().String(),
			Status:     customerv1.CustomerStatus_CUSTOMER_STATUS_UNSPECIFIED,
		},
	}

	_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extcustomer.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.Error(t, err)
}

func TestResolveByMobile_WhitespaceCustomerIDIsProtocolError(t *testing.T) {
	t.Parallel()

	fake := &fakeCustomerService{
		resolveResp: &customerv1.ResolveCustomerByMobileResponse{
			CustomerId: " " + uuid.New().String() + " ",
			Status:     customerv1.CustomerStatus_CUSTOMER_STATUS_ACTIVE,
		},
	}

	_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extcustomer.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.Error(t, err)
}

func TestResolveByMobile_MapsNotFound(t *testing.T) {
	t.Parallel()

	fake := &fakeCustomerService{
		resolveErr: status.Error(codes.NotFound, "missing"),
	}

	_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extcustomer.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.True(t, cerr.IsCode(err, cerr.CodeResourceNotFound))
}

func TestResolveByMobile_MapsUnavailable(t *testing.T) {
	t.Parallel()

	fake := &fakeCustomerService{
		resolveErr: status.Error(codes.Unavailable, "down"),
	}

	_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extcustomer.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.True(t, cerr.IsCode(err, cerr.CodeServiceUnavailable))
}

func TestDial_RequiresAddress(t *testing.T) {
	t.Parallel()

	_, err := Dial("", true)
	require.Error(t, err)
}

func TestDial_RequiresInsecureUntilTLSConfigured(t *testing.T) {
	t.Parallel()

	_, err := Dial("localhost:50051", false)
	require.Error(t, err)
}
