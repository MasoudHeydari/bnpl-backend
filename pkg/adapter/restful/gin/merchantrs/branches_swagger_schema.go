package merchantrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// BranchSchema is a schema-only merchant-branch list item.
type BranchSchema branchResponse

// BranchesListSuccessSchema is a schema-only success envelope for listing merchant branches.
//
// Data is the branch array returned by SerKeyset or SerPage. Meta.pagination
// carries keyset or page pagination state when present.
type BranchesListSuccessSchema struct {
	Data []BranchSchema      `json:"data"`
	Meta serdser.SuccessMeta `json:"meta"`
}
