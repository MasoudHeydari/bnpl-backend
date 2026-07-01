package config

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// Gin defines HTTP server settings used to construct the local Gin adapter.
// The struct is intended to be loaded once and reused as immutable startup
// configuration.
type Gin struct {
	Host           string   `yaml:"host"`
	Port           int      `yaml:"port"`
	TrustedProxies []string `yaml:"trusted_proxies"`
	Logger         bool     `yaml:"logger"`
	Recovery       bool     `yaml:"recovery"`
	Mode           string   `yaml:"mode"`
}

// NewEngine constructs a Gin HTTP engine configured with the selected mode
// and optional logging and recovery middleware. The returned engine is intended
// for setup during process startup before it begins serving requests.
func (g Gin) newEngine(provider trace.Provider) *gin.Engine {
	// TraceID is always the first middleware — it must run before recovery and
	// logging so that every log line and panic response can include the trace ID.
	middlewares := []gin.HandlerFunc{middleware.TraceID(provider)}

	if g.Recovery {
		middlewares = append(middlewares, gin.Recovery())
	}
	if g.Logger {
		middlewares = append(middlewares, gin.Logger())
	}

	return gin.New(g.Mode, g.TrustedProxies, middlewares...)
}

// GetListeningAddress formats the configured host and port into the address
// string expected by Gin's Run method.
func (g Gin) getListeningAddress() string {
	return fmt.Sprintf("%s:%d", g.Host, g.Port)
}
