package merchantrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// ProfileSchema is a schema-only merchant profile payload.
type ProfileSchema profileResponse

// GetSuccessSchema is a schema-only success envelope for the merchant profile.
type GetSuccessSchema struct {
	Data ProfileSchema                   `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}
