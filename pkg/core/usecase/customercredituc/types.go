package customercredituc

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// SummaryStatus is a REST-facing credit summary state distinct from persisted limit status.
type SummaryStatus string

const (
	// SummaryStatusActive indicates a current, usable credit limit assignment.
	SummaryStatusActive SummaryStatus = "active"

	// SummaryStatusNotAssigned indicates no credit limit has been assigned yet
	// (credit assessment has not completed). This is not an assessed zero balance.
	SummaryStatusNotAssigned SummaryStatus = "not_assigned"

	// SummaryStatusExpired indicates a historical limit exists but is not currently usable.
	SummaryStatusExpired SummaryStatus = "expired"
)

// Input identifies the authenticated external customer (Identity Service subject).
type Input struct {
	// ExternalCustomerID is the Identity Service subject mapped to customer_profiles.external_customer_id.
	ExternalCustomerID model.ExternalCustomerID
}

// Output carries the customer's current BNPL credit summary.
type Output struct {
	// CustomerID is the internal customer_profiles.id primary key.
	CustomerID model.CustomerID

	// AvailableAmountMinor is the available credit amount in minor currency units.
	AvailableAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency

	// Status is the current lifecycle status.
	Status SummaryStatus

	// LimitStatus is the persisted credit-limit status when a limit exists.
	LimitStatus *model.CreditAccountStatus

	// ExpiresAt is when the offer, session, or limit expires.
	ExpiresAt *time.Time
}
