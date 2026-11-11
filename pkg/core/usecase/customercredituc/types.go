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

// Input identifies the authenticated customer via gateway mobile for credit summary.
type Input struct {
	// MobileE164 is the gateway-injected authenticated mobile
	// (x-auth-mobile-number ← JWT auth_mobile_number), canonical E.164.
	MobileE164 string
}

// Output carries the customer's current BNPL credit summary.
type CreditUsage struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type Output struct {
	CreditUsage CreditUsage

	// CustomerID is the internal customer_profiles.id primary key.
	CustomerID model.CustomerID

	// LimitAmountMinor is the governed total credit capacity in minor units.
	LimitAmountMinor uint64

	// AvailableAmountMinor is remaining usable credit in minor currency units
	// (limit minus committed financed exposure; zero while new-credit use is blocked).
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
