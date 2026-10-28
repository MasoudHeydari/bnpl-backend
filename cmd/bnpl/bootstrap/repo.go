package bootstrap

import (
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/checkoutdecisionallocationrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/checkoutsessionrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/collectionscaserp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/contractrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/creditaccountrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/customerprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/customerrestrictionrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/decisionrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/defaultepisoderp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/healthrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/inboxrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentchargerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/merchantbranchprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/merchantprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/merchantterminalrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/otprp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/outboxrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/paymentattemptrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/policyassignmentimportrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/policyassignmentrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/repaymentreminderrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/sagarp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/shortlinkrp"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Repos holds repository adapters assembled for App.
//
// Each field implements a core repo port. Adapters are stateless factories;
// transactional scope comes from the shared Pool passed into use cases.
type Repos struct {

	// Outbox stores durable outbound integration messages.
	Outbox repo.Outbox

	// Health probes database readiness for dependency checks.
	Health repo.Health

	// Decision persists checkout decisions.
	Decision repo.Decision

	// CheckoutDecisionAllocation tracks decision-to-contract allocation.
	CheckoutDecisionAllocation repo.CheckoutDecisionAllocation

	// CheckoutSession persists merchant checkout handoff state and bindings.
	CheckoutSession repo.CheckoutSession

	// OTP persists reusable OTP challenges and cross-subject rate limits.
	OTP repo.OTP

	// Contract persists BNPL contracts and cancellation state.
	Contract repo.Contract

	// CustomerProfile maps Identity subjects to internal customer rows.
	CustomerProfile repo.CustomerProfile

	// CreditAccount stores current BNPL credit capacity and exposure.
	CreditAccount repo.CreditAccount

	// MerchantProfile stores merchant identities.
	MerchantProfile repo.MerchantProfile

	// MerchantBranchProfile stores merchant branch identities.
	MerchantBranchProfile repo.MerchantBranchProfile

	// MerchantTerminal stores external partner terminals, credentials, and
	// held issuance deliveries.
	MerchantTerminal repo.MerchantTerminal

	// Installment persists installment schedules and lifecycle state.
	Installment repo.Installment

	// InstallmentCharge persists installment charge rows.
	InstallmentCharge repo.InstallmentCharge

	// PaymentAttempt persists checkout and repayment payment attempts.
	PaymentAttempt repo.PaymentAttempt

	// PolicyAssignment stores runtime subject-to-policy assignment history.
	PolicyAssignment repo.PolicyAssignment

	// PolicyAssignmentImport stores durable bulk-import execution evidence.
	PolicyAssignmentImport repo.PolicyAssignmentImport

	// Saga persists durable Saga instances and step completions.
	Saga repo.Saga

	// Inbox stores inbound Financial Core message deduplication state.
	Inbox repo.Inbox

	// ShortLink stores modular opaque URL short links.
	ShortLink repo.ShortLink

	// RepaymentReminder stores customer-day repayment reminder requests.
	RepaymentReminder repo.RepaymentReminder

	// DefaultEpisode stores continuous customer default periods.
	DefaultEpisode repo.DefaultEpisode

	// CustomerRestriction stores durable customer capability restrictions.
	CustomerRestriction repo.CustomerRestriction

	// CollectionsCase stores human/legal collections workflow records.
	CollectionsCase repo.CollectionsCase
}

// NewRepos constructs the Postgres repository adapter set used by App.
func NewRepos() Repos {
	return Repos{
		Outbox:                     outboxrp.New(),
		Health:                     healthrp.New(),
		Decision:                   decisionrp.New(),
		CheckoutDecisionAllocation: checkoutdecisionallocationrp.New(),
		CheckoutSession:            checkoutsessionrp.New(),
		OTP:                        otprp.New(),
		Contract:                   contractrp.New(),
		CustomerProfile:            customerprofilerp.New(),
		CreditAccount:              creditaccountrp.New(),
		MerchantProfile:            merchantprofilerp.New(),
		MerchantBranchProfile:      merchantbranchprofilerp.New(),
		MerchantTerminal:           merchantterminalrp.New(),
		Installment:                installmentrp.New(),
		InstallmentCharge:          installmentchargerp.New(),
		PaymentAttempt:             paymentattemptrp.New(),
		PolicyAssignment:           policyassignmentrp.New(),
		PolicyAssignmentImport:     policyassignmentimportrp.New(),
		Saga:                       sagarp.New(),
		Inbox:                      inboxrp.New(),
		ShortLink:                  shortlinkrp.New(),
		RepaymentReminder:          repaymentreminderrp.New(),
		DefaultEpisode:             defaultepisoderp.New(),
		CustomerRestriction:        customerrestrictionrp.New(),
		CollectionsCase:            collectionscaserp.New(),
	}
}
