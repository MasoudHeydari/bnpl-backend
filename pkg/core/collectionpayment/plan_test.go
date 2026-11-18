package collectionpayment

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestPlanRejectsPrincipalWaiverAndDuplicateItems(t *testing.T) {
	customer, err := model.ParseCustomerID(uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	external, err := model.ParseExternalCustomerID(uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	contract, err := model.ParseContractID(uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	installment, err := model.ParseInstallmentID(uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	account, err := model.ParseCreditAccountID(uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	merchant, err := model.ParseExternalMerchantID(uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	assessment := Assessment{
		SourceType: "bnpl_installment", SourceID: installment.String(),
		SourceVersion: 1, PolicyReference: "assessment-v1", PolicyVersion: "v1",
		AssessedAt: time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC),
	}
	share := uuid.New()
	plan := Plan{
		ID: uuid.New(), CustomerID: customer, ExternalCustomerID: external,
		Currency: model.IRRCurrency, TotalMinor: 1000, Description: "repayment",
		Items: []Item{{
			ID: uuid.New(), InstallmentID: installment, ContractID: contract,
			CreditAccountID: account, CreditKind: model.PayInInstallmentsCreditKind,
			MerchantID: merchant, MerchantEntitlementID: "entitlement-registered",
			FundingShares: []FundingShare{{
				ID: share, Model: FundingPlatform,
				Funder:                  Party{Type: "platform"},
				FinancialRelationshipID: "financial-relationship-registered",
			}},
			Components: []Component{{
				ID: uuid.New(), Kind: PrincipalComponent, Assessment: assessment,
				GrossMinor: 1000, NetMinor: 1000,
				Allocations: []Allocation{{
					ID: uuid.New(), Purpose: ReceivableRecoveryPurpose,
					BeneficiaryType: "merchant", BeneficiaryID: merchant.String(),
					AmountMinor: 1000, RuleReference: "distribution-rule-registered",
					RuleVersion: "v1", EntitlementReference: "entitlement-registered",
					FundingShareID: &share,
				}},
			}},
		}},
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}

	plan.Items[0].Components[0].Adjustments = []Adjustment{{
		ID: uuid.New(), Kind: "waiver", AmountMinor: 100, Assessment: assessment,
	}}
	if err := plan.Validate(); err == nil {
		t.Fatal("previously recognized principal was waived")
	}

	plan.Items[0].Components[0].Adjustments = nil
	plan.TotalMinor = 900
	if err := plan.Validate(); err == nil {
		t.Fatal("inconsistent total accepted")
	}

	plan.TotalMinor = 1000
	plan.Items = append(plan.Items, plan.Items[0])
	plan.Items[1].ID = uuid.New()
	if err := plan.Validate(); err == nil {
		t.Fatal("duplicated installment accepted")
	}
}
