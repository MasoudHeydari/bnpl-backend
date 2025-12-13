package config

import (
	"fmt"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/restful/gin"
)

type Gin struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Logger   bool   `yaml:"logger"`
	Recovery bool   `yaml:"recovery"`
	Mode     string `yaml:"mode"`
}

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

func (g Gin) GetListeningAddress() string {
	return fmt.Sprintf("%s:%d", g.Host, g.Port)
}
