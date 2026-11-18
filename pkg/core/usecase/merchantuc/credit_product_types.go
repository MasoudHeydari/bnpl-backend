package merchantuc

import "github.com/parstis/bnpl-backend/pkg/core/model"

// SetCreditProductEnabledInput enables or disables one credit product for a merchant.
//
// MerchantProfileID is the internal merchant_profiles.id primary key; it is not
// the Identity Service merchant subject. ActorSubjectID is the authenticated
// administrator (x-auth-subject-id) recorded in the audit event.
type SetCreditProductEnabledInput struct {
	// ActorSubjectID is the authenticated administrator performing the change.
	ActorSubjectID model.SubjectID
	// MerchantProfileID is the internal merchant profile primary key.
	MerchantProfileID model.MerchantProfileID
	// CreditKind selects pay_in_4 or pay_next_month.
	CreditKind model.CreditKind
	// Enabled is the desired eligibility state.
	Enabled bool
}

// SetCreditProductEnabledOutput reports the effective eligibility after the command.
type SetCreditProductEnabledOutput struct {
	// MerchantProfileID is the internal merchant profile primary key.
	MerchantProfileID model.MerchantProfileID
	// CreditKind is the affected credit product.
	CreditKind model.CreditKind
	// Enabled is the effective eligibility after the command.
	Enabled bool
	// Changed is false when the merchant already had the requested state
	// (idempotent replay); no write and no audit event are produced then.
	Changed bool
}

// ListCreditProductsInput identifies the merchant whose eligibility is read.
type ListCreditProductsInput struct {
	// MerchantProfileID is the internal merchant profile primary key.
	MerchantProfileID model.MerchantProfileID
}

// CreditProductEligibility is the effective eligibility of one credit product.
type CreditProductEligibility struct {
	// CreditKind is the credit product.
	CreditKind model.CreditKind
	// Enabled is the effective eligibility (legacy pay_in_4 defaults to enabled
	// when absent; pay_next_month defaults to disabled).
	Enabled bool
}

// ListCreditProductsOutput lists the effective eligibility of every credit product.
type ListCreditProductsOutput struct {
	// MerchantProfileID is the internal merchant profile primary key.
	MerchantProfileID model.MerchantProfileID
	// Products lists pay_in_4 followed by pay_next_month.
	Products []CreditProductEligibility
}
