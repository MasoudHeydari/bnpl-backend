// Package collectionpayment defines FC-independent, immutable BNPL
// collection assessments. Financial Core owns all monetary truth.
package collectionpayment

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ComponentKind classifies a customer obligation, not its beneficiary.
type ComponentKind string
const (
	PrincipalComponent ComponentKind = "principal"
	LateFeeComponent ComponentKind = "late_fee"
	ServiceFeeComponent ComponentKind = "service_fee"
	TaxComponent ComponentKind = "tax"
	BaseAmountComponent ComponentKind = "base_amount"
)

func (k ComponentKind) IsValid() bool {
	switch k {
	case PrincipalComponent,LateFeeComponent,ServiceFeeComponent,TaxComponent,BaseAmountComponent:
		return true
	default:
		return false
	}
}

// AllocationPurpose names an authorized FC posting purpose, never an account.
type AllocationPurpose string
const (
	ReceivableRecoveryPurpose AllocationPurpose = "receivable_recovery"
	MerchantPayablePurpose AllocationPurpose = "merchant_payable_on_collection"
	PlatformRevenuePurpose AllocationPurpose = "platform_revenue_on_collection"
	FunderPayablePurpose AllocationPurpose = "funder_payable_on_collection"
	TaxLiabilityPurpose AllocationPurpose = "tax_liability_on_collection"
	PreRecognitionCashPurpose AllocationPurpose = "pre_recognition_cash"
)
func (p AllocationPurpose) IsValid() bool {
	switch p {
	case ReceivableRecoveryPurpose,MerchantPayablePurpose,PlatformRevenuePurpose,
		FunderPayablePurpose,TaxLiabilityPurpose,PreRecognitionCashPurpose:
		return true
	default:
		return false
	}
}

// Assessment snapshots the BNPL business rule used to assess a charge.
// FC independently validates policy, relationship and distribution authority.
type Assessment struct {
	SourceType string
	SourceID string
	SourceVersion int64
	PolicyReference string
	PolicyVersion string
	AssessedAt time.Time
}

// Adjustment is a discount/waiver on an assessed component. Never apply it
// to an already-recognized principal receivable.
type Adjustment struct {
	ID uuid.UUID
	Kind string
	AmountMinor uint64
	Assessment Assessment
}

// Allocation contains an exact economic amount and an FC-provisioned rule.
// EntitlementReference is opaque: BNPL must not invent Claim/Recognition IDs.
type Allocation struct {
	ID uuid.UUID
	Purpose AllocationPurpose
	BeneficiaryType string
	BeneficiaryID string
	AmountMinor uint64
	RuleReference string
	RuleVersion string
	EntitlementReference string
	FundingShareID *uuid.UUID
}

// Component records original, adjusted and beneficiary amounts separately.
type Component struct {
	ID uuid.UUID
	Kind ComponentKind
	Assessment Assessment
	GrossMinor uint64
	Adjustments []Adjustment
	NetMinor uint64
	Allocations []Allocation
}

// Item is one BNPL installment from a single contract and credit account.
type Item struct {
	ID uuid.UUID
	InstallmentID model.InstallmentID
	ContractID model.ContractID
	CreditAccountID model.CreditAccountID
	CreditKind model.CreditKind
	MerchantID model.ExternalMerchantID
	MerchantEntitlementID string
	FinancialRelationshipID string
	Components []Component
}

// Plan is a customer-accepted repayment snapshot. It can cover multiple
// merchants and credit kinds; a Checkout itself still uses exactly one kind.
type Plan struct {
	ID uuid.UUID
	ExternalCustomerID model.ExternalCustomerID
	CustomerID model.CustomerID
	Currency model.Currency
	Items []Item
	TotalMinor uint64
	Description string
}

// Validate checks immutable IDs and monetary conservation, not FC authority.
// Every total uses checked arithmetic within signed-int64 wire-unit bounds.
func (p Plan) Validate() error {
	const op = "collectionpayment.Plan.Validate"
	if p.ID == uuid.Nil || p.CustomerID.IsZero() || p.ExternalCustomerID.IsZero() || !p.Currency.IsValid() {
		return fmt.Errorf("%s: invalid batch or customer identity", op)
	}
	if len(p.Items) == 0 || strings.TrimSpace(p.Description) == "" {
		return fmt.Errorf("%s: items and description are required", op)
	}

	seen := make(map[uuid.UUID]struct{})
	installments := make(map[model.InstallmentID]struct{})
	var total uint64

	for _, item := range p.Items {
		if item.ID == uuid.Nil || item.InstallmentID.IsZero() || item.ContractID.IsZero() ||
			item.CreditAccountID.IsZero() || !item.CreditKind.IsValid() ||
			item.MerchantID.IsZero() || strings.TrimSpace(item.MerchantEntitlementID) == "" ||
			strings.TrimSpace(item.FinancialRelationshipID) == "" || len(item.Components) == 0 {
			return fmt.Errorf("%s: incomplete installment or financial relationship", op)
		}
		if _, ok := installments[item.InstallmentID]; ok {
			return fmt.Errorf("%s: duplicate installment", op)
		}
		installments[item.InstallmentID] = struct{}{}
		if err := uniqueID(seen, item.ID); err != nil { return err }

		for _, component := range item.Components {
			if err := uniqueID(seen, component.ID); err != nil { return err }
			if !component.Kind.IsValid() || component.GrossMinor == 0 || component.GrossMinor > math.MaxInt64 {
				return fmt.Errorf("%s: invalid component kind or gross", op)
			}
			if err := validateAssessment(component.Assessment); err != nil { return err }

			// FC has already recognized this receivable at purchase activation.
			// A collection-time waiver cannot quietly extinguish principal.
			if component.Kind == PrincipalComponent && len(component.Adjustments) > 0 {
				return fmt.Errorf("%s: previously recognized principal cannot be adjusted", op)
			}
			var deductions uint64
			for _, adjustment := range component.Adjustments {
				if err := uniqueID(seen, adjustment.ID); err != nil { return err }
				if adjustment.Kind != "discount" && adjustment.Kind != "waiver" {
					return fmt.Errorf("%s: invalid adjustment kind", op)
				}
				if err := validateAssessment(adjustment.Assessment); err != nil { return err }
				var err error
				deductions, err = checkedAdd(deductions, adjustment.AmountMinor)
				if err != nil { return err }
			}
			if deductions > component.GrossMinor || component.NetMinor != component.GrossMinor-deductions {
				return fmt.Errorf("%s: net amount mismatch", op)
			}

			var allocated uint64
			for _, allocation := range component.Allocations {
				if err := uniqueID(seen, allocation.ID); err != nil { return err }
				if !allocation.Purpose.IsValid() || strings.TrimSpace(allocation.RuleReference) == "" ||
					strings.TrimSpace(allocation.RuleVersion) == "" ||
					strings.TrimSpace(allocation.BeneficiaryType) == "" || allocation.AmountMinor == 0 {
					return fmt.Errorf("%s: unprovisioned or invalid allocation", op)
				}
				if allocation.Purpose == ReceivableRecoveryPurpose &&
					strings.TrimSpace(allocation.EntitlementReference) == "" {
					return fmt.Errorf("%s: receivable recovery needs FC entitlement reference", op)
				}
				var err error
				allocated, err = checkedAdd(allocated, allocation.AmountMinor)
				if err != nil { return err }
			}
			if allocated != component.NetMinor {
				return fmt.Errorf("%s: component allocation sum mismatch", op)
			}
			var err error
			total, err = checkedAdd(total, component.NetMinor)
			if err != nil { return err }
		}
	}
	if total == 0 || total != p.TotalMinor {
		return fmt.Errorf("%s: positive payment amount must equal all net components", op)
	}
	return nil
}

func uniqueID(seen map[uuid.UUID]struct{}, id uuid.UUID) error {
	if id == uuid.Nil { return fmt.Errorf("immutable id is required") }
	if _, exists := seen[id]; exists { return fmt.Errorf("duplicate immutable id %s", id) }
	seen[id] = struct{}{}
	return nil
}

func checkedAdd(a, b uint64) (uint64,error) {
	if a > math.MaxInt64 || b > math.MaxInt64 || b > math.MaxInt64-a {
		return 0, fmt.Errorf("collection amount exceeds int64 minor units")
	}
	return a+b,nil
}

func validateAssessment(a Assessment) error {
	if strings.TrimSpace(a.SourceType)=="" || strings.TrimSpace(a.SourceID)=="" ||
		a.SourceVersion<1 || strings.TrimSpace(a.PolicyReference)=="" ||
		strings.TrimSpace(a.PolicyVersion)=="" || a.AssessedAt.IsZero() {
		return fmt.Errorf("assessment source, policy and time are required")
	}
	return nil
}
