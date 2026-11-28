package merchant

import "github.com/parstis/bnpl-backend/pkg/core/model"

type referenceDTO struct {
	MerchantID            model.ExternalMerchantID             `json:"merchant_id"`
	RequestID             model.MerchantParticipationRequestID `json:"request_id"`
	BusinessLine          string                               `json:"business_line"`
	Environment           string                               `json:"environment"`
	TradeName             string                               `json:"trade_name"`
	BaseApplicationStatus string                               `json:"base_application_status"`
	SelfDeclaration       *selfDeclarationDTO                  `json:"self_declaration"`
}

type selfDeclarationDTO struct {
	MaxAmountMinor    uint64         `json:"max_amount_minor"`
	MaxAmountCurrency model.Currency `json:"max_amount_currency"`
	MaxTenureMonths   uint           `json:"max_tenure_months"`
}

type progressDTO struct {
	EventID          string     `json:"event_id"`
	OwnerVersion     int64      `json:"owner_version"`
	Status           string     `json:"status"`
	AccountReference string     `json:"external_account_reference"`
	Issues           []issueDTO `json:"issues"`
}

type issueDTO struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type snapshotDTO struct {
	RequestID        model.MerchantParticipationRequestID `json:"request_id"`
	Code             string                               `json:"code"`
	Environment      string                               `json:"environment"`
	Status           string                               `json:"status"`
	OwnerVersion     int64                                `json:"owner_version"`
	AccountReference *string                              `json:"external_account_reference"`
}
