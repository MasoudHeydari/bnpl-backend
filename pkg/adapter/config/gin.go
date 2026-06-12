package config

import (
	"fmt"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/restful/gin"
)

// Gin defines HTTP server settings used to construct the local Gin adapter.
// The struct is intended to be loaded once and reused as immutable startup
// configuration.
type Gin struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Logger   bool   `yaml:"logger"`
	Recovery bool   `yaml:"recovery"`
	Mode     string `yaml:"mode"`
}

// NewEngine constructs a Gin HTTP engine configured with the selected mode
// and optional logging and recovery middleware. The returned engine is intended
// for setup during process startup before it begins serving requests.
func (g Gin) NewEngine() *gin.Engine {
	middlewares := make([]gin.HandlerFunc, 0, 2)
	if g.Logger {
		middlewares = append(middlewares, gin.Logger())
	}
	if g.Recovery {
		middlewares = append(middlewares, gin.Recovery())
	}

	return gin.New(g.Mode, middlewares...)
}

// GetListeningAddress formats the configured host and port into the address
// string expected by Gin's Run method.
func (g Gin) GetListeningAddress() string {
	return fmt.Sprintf("%s:%d", g.Host, g.Port)
}
