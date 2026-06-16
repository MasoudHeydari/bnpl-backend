package config

import "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"

type Infrastructure struct {
	Trace Trace
}

type Trace struct{}

func (t Trace) newTraceInfrastructure() (*trace.Provider, error) {
	return trace.NewULIDProvider(), nil
}
