package repaymentbatchrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// Schema-only wrappers for the generated Swagger API contract.
type CreateBatchRequestSchema createRequest
type BatchStatusSchema statusResponse
type StartBatchSchema startResponse

type BatchSuccessSchema struct {
	Data BatchStatusSchema `json:"data"`
	Meta serdser.SuccessMeta `json:"meta"`
}
type StartBatchSuccessSchema struct {
	Data StartBatchSchema `json:"data"`
	Meta serdser.SuccessMeta `json:"meta"`
}
