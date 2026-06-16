package bootstrap

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// Infrastructure holds the runtime adapters assembled for the application.
type Infrastructure struct {
	Trace trace.Provider
}

// NewInfrastructures constructs the infrastructure adapters required by App.
func (a *App) NewInfrastructures() (Infrastructure, error) {
	traceProvider, err := a.Config.NewTraceInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("initialize trace: %w", err)
	}

	return Infrastructure{
		Trace: traceProvider,
	}, nil
}
