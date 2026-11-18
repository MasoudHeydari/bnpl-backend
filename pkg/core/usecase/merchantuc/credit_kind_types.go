package merchantuc

import "github.com/parstis/bnpl-backend/pkg/core/model"

// SetCreditKindEnabledInput enables or disables one credit kind for a merchant.
//
// MerchantProfileID is the internal merchant_profiles.id primary key; it is not
// the Identity Service merchant subject. ActorSubjectID is the authenticated
// administrator (x-auth-subject-id) recorded in the audit event.
type SetCreditKindEnabledInput struct {
	// ActorSubjectID is the authenticated administrator performing the change.
	ActorSubjectID model.SubjectID
	// MerchantProfileID is the internal merchant profile primary key.
	MerchantProfileID model.MerchantProfileID
	// CreditKind selects pay_in_installments or pay_next_month.
	CreditKind model.CreditKind
	// Enabled is the desired eligibility state.
	Enabled bool
}

// SetCreditKindEnabledOutput reports the effective eligibility after the command.
type SetCreditKindEnabledOutput struct {
	// MerchantProfileID is the internal merchant profile primary key.
	MerchantProfileID model.MerchantProfileID
	// CreditKind is the affected credit kind.
	CreditKind model.CreditKind
	// Enabled is the effective eligibility after the command.
	Enabled bool
	// Changed is false when the merchant already had the requested state
	// (idempotent replay); no write and no audit event are produced then.
	Changed bool
}

// ListCreditKindsInput identifies the merchant whose eligibility is read.
type ListCreditKindsInput struct {
	// MerchantProfileID is the internal merchant profile primary key.
	MerchantProfileID model.MerchantProfileID
}

// CreditKindEligibility is the effective eligibility of one credit kind.
type CreditKindEligibility struct {
	// CreditKind is the credit kind.
	CreditKind model.CreditKind
	// Enabled is the effective eligibility. Both kinds default to disabled when
	// no eligibility row exists.
	Enabled bool
}

// ListCreditKindsOutput lists the effective eligibility of every credit kind.
type ListCreditKindsOutput struct {
	// MerchantProfileID is the internal merchant profile primary key.
	MerchantProfileID model.MerchantProfileID
	// CreditKinds lists pay_in_installments followed by pay_next_month.
	CreditKinds []CreditKindEligibility
}
