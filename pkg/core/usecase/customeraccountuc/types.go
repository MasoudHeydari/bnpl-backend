package customeraccountuc

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
)

// ListActiveContractsInput identifies the authenticated external customer and keyset page.
//
// IdentitySubjectID is the Identity Service subject ID (x-auth-subject-id /
// customer_profiles.identity_subject_id), not customer_profiles.id.
type ListActiveContractsInput struct {
	// IdentitySubjectID is the Identity Service subject mapped to customer_profiles.identity_subject_id.
	IdentitySubjectID model.IdentitySubjectID
	// Page carries pagination request or response metadata.
	Page pagination.KeysetRequest
}

// ActiveContract is an API-facing current purchased-order summary (active or canceling).
type ActiveContract struct {
	// ID uniquely identifies the record.
	ID model.ContractID
	// MerchantProfileID identifies the merchant profile.
	MerchantProfileID model.MerchantProfileID
	// MerchantBranchProfileID identifies the merchant branch profile when present.
	MerchantBranchProfileID *model.MerchantBranchProfileID
	// RequestedAmountMinor is the requested amount in minor currency units.
	RequestedAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// InstallmentCount is the number of installments in the schedule.
	InstallmentCount int
	// Status is the current lifecycle status.
	Status model.ContractStatus
	// CreatedAt is when the record was created.
	CreatedAt time.Time
}

// ListActiveContractsOutput carries the current contract page and keyset pagination metadata.
type ListActiveContractsOutput struct {
	// Contracts lists the matching contracts.
	Contracts []ActiveContract
	// Page carries pagination request or response metadata.
	Page pagination.KeysetMeta
}

// GetContractInstallmentsInput identifies the authenticated external customer and target contract.
//
// IdentitySubjectID is the Identity Service subject ID (x-auth-subject-id /
// customer_profiles.identity_subject_id), not customer_profiles.id.
type GetContractInstallmentsInput struct {
	// IdentitySubjectID is the Identity Service subject mapped to customer_profiles.identity_subject_id.
	IdentitySubjectID model.IdentitySubjectID
	// ContractID identifies the contract.
	ContractID model.ContractID
}

// InstallmentView is the API-facing installment schedule row.
type InstallmentView struct {
	// ID uniquely identifies the record.
	ID model.InstallmentID
	// InstallmentOrder is the 1-based order of the installment in the schedule.
	InstallmentOrder int
	// TotalAmountMinor is the total amount in minor currency units.
	TotalAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// DueAt is when the installment becomes due.
	DueAt time.Time
	// Status is the current lifecycle status.
	Status model.InstallmentStatus
	// PaidAt is when payment occurred, if applicable.
	PaidAt *time.Time
}

// NextPayableInstallmentView identifies the next installment that may be paid
// under sequential repayment rules, including the current customer-payable amount.
type NextPayableInstallmentView struct {
	// ID uniquely identifies the record.
	ID model.InstallmentID
	// InstallmentOrder is the 1-based order of the installment in the schedule.
	InstallmentOrder int
	// PayableAmountMinor is the current customer-payable amount in minor currency units.
	PayableAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// DueAt is when the installment becomes due.
	DueAt time.Time
	// Status is the current lifecycle status.
	Status model.InstallmentStatus
}

// GetContractInstallmentsOutput carries the contract order summary and installment schedule.
type GetContractInstallmentsOutput struct {
	// ContractID identifies the contract.
	ContractID model.ContractID
	// RequestedAmountMinor is the requested amount in minor currency units.
	RequestedAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// Status is the current lifecycle status.
	Status model.ContractStatus
	// CreatedAt is when the record was created.
	CreatedAt time.Time
	// NextPayable describes the next payable installment when one exists.
	NextPayable *NextPayableInstallmentView
	// Installments lists the installment views or records.
	Installments []InstallmentView
}

// ListUnpaidInstallmentsInput identifies the authenticated external customer and keyset page.
//
// IdentitySubjectID is the Identity Service subject ID (x-auth-subject-id /
// customer_profiles.identity_subject_id), not customer_profiles.id.
type ListUnpaidInstallmentsInput struct {
	// IdentitySubjectID is the Identity Service subject mapped to customer_profiles.identity_subject_id.
	IdentitySubjectID model.IdentitySubjectID
	// Page carries pagination request or response metadata.
	Page pagination.KeysetRequest
}

// UnpaidInstallment is an API-facing unpaid installment summary.
type UnpaidInstallment struct {
	// ID uniquely identifies the record.
	ID model.InstallmentID
	// ContractID identifies the contract.
	ContractID model.ContractID
	// MerchantProfileID identifies the merchant profile.
	MerchantProfileID model.MerchantProfileID
	// MerchantBranchProfileID identifies the merchant branch profile when present.
	MerchantBranchProfileID *model.MerchantBranchProfileID
	// InstallmentOrder is the 1-based order of the installment in the schedule.
	InstallmentOrder int
	// TotalAmountMinor is the total amount in minor currency units.
	TotalAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// DueAt is when the installment becomes due.
	DueAt time.Time
	// Status is the current lifecycle status.
	Status model.InstallmentStatus
	// PaidAt is when payment occurred, if applicable.
	PaidAt *time.Time
}

// ListUnpaidInstallmentsOutput carries the unpaid installment page and keyset pagination metadata.
type ListUnpaidInstallmentsOutput struct {
	// Installments lists the installment views or records.
	Installments []UnpaidInstallment
	// Page carries pagination request or response metadata.
	Page pagination.KeysetMeta
}

// ListPaymentsInput identifies the authenticated external customer and keyset page.
//
// IdentitySubjectID is the Identity Service subject ID (x-auth-subject-id /
// customer_profiles.identity_subject_id), not customer_profiles.id.
type ListPaymentsInput struct {
	// IdentitySubjectID is the Identity Service subject mapped to customer_profiles.identity_subject_id.
	IdentitySubjectID model.IdentitySubjectID
	// Page carries pagination request or response metadata.
	Page pagination.KeysetRequest
}

// Payment is an API-facing finalized installment payment attempt summary.
type Payment struct {
	// ID uniquely identifies the record.
	ID model.PaymentAttemptID
	// ContractID identifies the contract.
	ContractID model.ContractID
	// InstallmentID identifies the installment.
	InstallmentID model.InstallmentID
	// AmountMinor is the amount in minor currency units.
	AmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// Status is the current lifecycle status.
	Status model.PaymentAttemptState
	// Purpose classifies the payment purpose.
	Purpose model.FinancialCorePaymentPurpose
	// CreatedAt is when the record was created.
	CreatedAt time.Time
	// CompletedAt is when the payment attempt completed, if finalized.
	CompletedAt *time.Time
}

// ListPaymentsOutput carries the payment page and keyset pagination metadata.
type ListPaymentsOutput struct {
	// Payments lists the payment records.
	Payments []Payment
	// Page carries pagination request or response metadata.
	Page pagination.KeysetMeta
}
