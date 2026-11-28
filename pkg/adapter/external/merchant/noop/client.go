package noop

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var _ merchant.Client = (*Client)(nil)

// Client never fabricates a reference or a successful report.
type Client struct{}

func New() *Client { return &Client{} }

func (*Client) GetParticipation(context.Context, model.MerchantParticipationRequestID) (merchant.Reference, error) {
	return merchant.Reference{}, merchant.ErrBlocked
}

func (*Client) ReportProgress(context.Context, merchant.Progress) (merchant.Snapshot, error) {
	return merchant.Snapshot{}, merchant.ErrBlocked
}

func (*Client) GetOutletSetupEligibility(context.Context, model.ExternalMerchantID, model.ExternalBranchID) (merchant.OutletEligibility, error) {
	return merchant.OutletEligibility{}, merchant.ErrBlocked
}
