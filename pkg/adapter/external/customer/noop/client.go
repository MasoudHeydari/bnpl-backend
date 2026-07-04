package noop

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var _ extcustomer.Resolver = (*Client)(nil)

// customerNamespace derives deterministic local CRS customer ids from mobiles.
var customerNamespace = uuid.MustParse("6f0c3a56-8d52-4c43-9d51-1f3b3e0a7c10")

// Client resolves mobiles to deterministic local customers without calling the
// Customer Reference Service.
type Client struct{}

// New constructs a noop Customer Reference Service client.
func New() *Client {
	return &Client{}
}

// ResolveByMobile returns a stable active customer derived from the mobile, so
// the same mobile always resolves to the same ExternalCustomerID.
func (c *Client) ResolveByMobile(
	_ context.Context,
	input extcustomer.ResolveByMobileInput,
) (extcustomer.ResolveByMobileOutput, error) {
	const op = "noop.Client.ResolveByMobile"

	mobile := strings.TrimSpace(input.MobileE164)
	if mobile == "" {
		return extcustomer.ResolveByMobileOutput{}, fmt.Errorf("%s: mobile is required", op)
	}

	externalCustomerID, err := model.ParseExternalCustomerID(
		uuid.NewSHA1(customerNamespace, []byte(mobile)).String(),
	)
	if err != nil {
		return extcustomer.ResolveByMobileOutput{}, fmt.Errorf("%s: derive customer id: %w", op, err)
	}

	return extcustomer.ResolveByMobileOutput{
		ExternalCustomerID: externalCustomerID,
		Status:             extcustomer.ActiveStatus,
	}, nil
}
