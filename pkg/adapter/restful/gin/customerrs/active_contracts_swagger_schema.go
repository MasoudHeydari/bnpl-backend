package customerrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// ContractSchema is a schema-only current-contract list item.
type ContractSchema contractResponse

// ActiveContractsListSuccessSchema is a schema-only success envelope for listing current contracts.
//
// Data is the contract array returned by SerKeyset. Meta.pagination carries
// keyset pagination state when present.
type ActiveContractsListSuccessSchema struct {
	Data []ContractSchema          `json:"data"`
	Meta serdser.SuccessMetaKeyset `json:"meta"`
}
