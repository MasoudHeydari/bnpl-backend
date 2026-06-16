package config

import (
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"
	coreclock "github.com/parstis/bnpl-backend/pkg/core/clock"
)

type Infrastructure struct {
	Trace Trace
	Clock Clock
}

type Trace struct{}

func (t Trace) newTraceInfrastructure() (*trace.Provider, error) {
	return trace.NewULIDProvider(), nil
}

type Clock struct {
	Location         string `yaml:"location"`
	BusinessLocation string `yaml:"business_location"`
}

func (c Clock) newSystemClockInfrastructure() (coreclock.Clock, error) {
	location, err := time.LoadLocation(c.Location)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid infrastructure.clock.location %q: %w",
			c.Location,
			err,
		)
	}

	businessLocation, err := time.LoadLocation(c.BusinessLocation)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid infrastructure.clock.business_location %q: %w",
			c.BusinessLocation,
			err,
		)
	}

	return clock.NewSystem(location, businessLocation), nil
}
