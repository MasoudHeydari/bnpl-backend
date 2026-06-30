package routes

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/healthrs"

// RegisterOptions groups registration options for HTTP resources.
//
// Each resource that exposes configurable registration options should add its
// own options here and receive them during route registration.
type RegisterOptions struct {
	// Health configures registration of the health resources.
	Health healthrs.RegisterOptions
}
