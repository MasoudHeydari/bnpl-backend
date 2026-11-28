package grpc

import (
	"context"
	"fmt"
	"time"

	grpcclient "google.golang.org/grpc"

	merchantv1 "github.com/parstis/bnpl-backend/gen/proto/merchant/participation/v1"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const circuitMerchant = "external.merchant.participation"

var _ merchant.Client = (*Client)(nil)

// Client invokes the private Merchant contract on the shared gRPC transport.
type Client struct {
	client  merchantv1.MerchantParticipationServiceClient
	timeout time.Duration
	breaker circuitbreaker.CircuitBreaker
}

// New requires the connection, configured deadline and shared circuit breaker.
func New(conn grpcclient.ClientConnInterface, timeout time.Duration, breaker circuitbreaker.CircuitBreaker) (*Client, error) {
	const op = "grpc.New"

	if conn == nil || timeout <= 0 || breaker == nil {
		return nil, fmt.Errorf("%s: connection, positive timeout and circuit breaker are required", op)
	}

	return &Client{
		client:  merchantv1.NewMerchantParticipationServiceClient(conn),
		timeout: timeout,
		breaker: breaker,
	}, nil
}

func (c *Client) execute(ctx context.Context, op string, call func(context.Context) error) error {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var contractErr error
	err := c.breaker.Execute(callCtx, circuitMerchant, func(ctx context.Context) error {
		err := call(ctx)
		if isContractError(err) {
			contractErr = err
			return nil
		}

		return err
	})
	if err != nil {
		return mapError(op, err)
	}

	return mapError(op, contractErr)
}

// GetParticipation reads the authoritative current declaration.
func (c *Client) GetParticipation(ctx context.Context, id model.MerchantParticipationRequestID) (merchant.Reference, error) {
	const op = "grpc.Client.GetParticipation"

	request := &merchantv1.GetParticipationRequestRequest{RequestId: id.String()}
	var response *merchantv1.GetParticipationRequestResponse
	err := c.execute(ctx, op, func(ctx context.Context) error {
		var err error
		response, err = c.client.GetParticipationRequest(ctx, request)
		return err
	})
	if err != nil {
		return merchant.Reference{}, err
	}

	return toReference(response)
}

// ReportProgress sends the exact immutable persisted report on each attempt.
func (c *Client) ReportProgress(ctx context.Context, progress merchant.Progress) (merchant.Snapshot, error) {
	const op = "grpc.Client.ReportProgress"

	request, err := toProgressRequest(progress)
	if err != nil {
		return merchant.Snapshot{}, err
	}

	var response *merchantv1.ParticipationSnapshot
	err = c.execute(ctx, op, func(ctx context.Context) error {
		var err error
		response, err = c.client.ReportParticipationProgress(ctx, request)
		return err
	})
	if err != nil {
		return merchant.Snapshot{}, err
	}

	return toSnapshot(response)
}

// GetOutletSetupEligibility is a reference query, not user authorization.
func (c *Client) GetOutletSetupEligibility(ctx context.Context, merchantID model.ExternalMerchantID, outletID model.ExternalBranchID) (merchant.OutletEligibility, error) {
	const op = "grpc.Client.GetOutletSetupEligibility"

	request := &merchantv1.GetOutletSetupEligibilityRequest{
		MerchantId: merchantID.String(),
		OutletId:   outletID.String(),
	}
	var response *merchantv1.GetOutletSetupEligibilityResponse
	err := c.execute(ctx, op, func(ctx context.Context) error {
		var err error
		response, err = c.client.GetOutletSetupEligibility(ctx, request)
		return err
	})
	if err != nil {
		return merchant.OutletEligibility{}, err
	}

	if response == nil || response.GetEnvironment() != merchantv1.ParticipationEnvironment_PARTICIPATION_ENVIRONMENT_SANDBOX {
		return merchant.OutletEligibility{}, fmt.Errorf("%s: invalid outlet eligibility response: %w", op, merchant.ErrBlocked)
	}

	return merchant.OutletEligibility{
		Eligible:    response.GetEligibleForSandboxSetup(),
		Environment: "SANDBOX",
	}, nil
}
