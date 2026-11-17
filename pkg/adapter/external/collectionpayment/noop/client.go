// Package noop disables CollectionPayment v2 without simulating a payment.
package noop

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/collectionpayment"
	external "github.com/parstis/bnpl-backend/pkg/core/external/collectionpayment"
)

type Client struct{}

var _ external.Client = (*Client)(nil)

func New() *Client { return &Client{} }
func (c *Client) Prepare(context.Context, collectionpayment.Plan, string) (external.Result, error) {
	return external.Result{}, fmt.Errorf("Financial Core CollectionPayment v2 is disabled")
}
func (c *Client) Start(context.Context, string, string, string, external.ReturnURLs, int64) (external.Result, error) {
	return external.Result{}, fmt.Errorf("Financial Core CollectionPayment v2 is disabled")
}
func (c *Client) GetByID(context.Context, string) (external.Result, error) {
	return external.Result{}, fmt.Errorf("Financial Core CollectionPayment v2 is disabled")
}
func (c *Client) GetBySource(context.Context, uuid.UUID) (external.Result, error) {
	return external.Result{}, fmt.Errorf("Financial Core CollectionPayment v2 is disabled")
}
