package healthcheckuc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

type healthcheckucDeps struct {
	lifecycleState    *mock.LifecycleState
	dependencyChecker *mock.DependencyChecker
}

func newHealthCheckUCDeps(t *testing.T) healthcheckucDeps {
	t.Helper()

	return healthcheckucDeps{
		lifecycleState:    mock.NewLifecycleState(t),
		dependencyChecker: mock.NewDependencyChecker(t),
	}
}

func (d healthcheckucDeps) newUseCase() *UseCase {
	return New(d.lifecycleState, d.dependencyChecker)
}

func TestLivenessReturnsNilInNormalState(t *testing.T) {
	t.Parallel()

	deps := newHealthCheckUCDeps(t)
	deps.lifecycleState.EXPECT().IsShutdownTimedOut().Return(false).Once()

	uc := deps.newUseCase()

	err := uc.Liveness(context.Background())
	require.NoError(t, err)
}

func TestLivenessReturnsErrorWhenShutdownTimedOut(t *testing.T) {
	t.Parallel()

	deps := newHealthCheckUCDeps(t)
	deps.lifecycleState.EXPECT().IsShutdownTimedOut().Return(true).Once()

	uc := deps.newUseCase()

	err := uc.Liveness(context.Background())
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeServiceUnavailable))
	require.ErrorContains(t, err, "error.application_shutdown_timed_out")
}

func TestReadinessReturnsNilWhenDependenciesAreHealthy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newHealthCheckUCDeps(t)
	deps.lifecycleState.EXPECT().IsDraining().Return(false).Once()
	deps.dependencyChecker.EXPECT().Check(ctx).Return(nil).Once()

	uc := deps.newUseCase()

	err := uc.Readiness(ctx)
	require.NoError(t, err)
}

func TestReadinessReturnsErrorWhenDependencyCheckFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	checkErr := errors.New("connection refused")
	deps := newHealthCheckUCDeps(t)
	deps.lifecycleState.EXPECT().IsDraining().Return(false).Once()
	deps.dependencyChecker.EXPECT().Check(ctx).Return(checkErr).Once()

	uc := deps.newUseCase()

	err := uc.Readiness(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, checkErr)
}

func TestReadinessReturnsErrorWhenDraining(t *testing.T) {
	t.Parallel()

	deps := newHealthCheckUCDeps(t)
	deps.lifecycleState.EXPECT().IsDraining().Return(true).Once()

	uc := deps.newUseCase()

	err := uc.Readiness(context.Background())
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeServiceUnavailable))
	require.ErrorContains(t, err, "draining")
}

func TestReadinessFailsBeforeLivenessDuringDrain(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newHealthCheckUCDeps(t)
	deps.lifecycleState.EXPECT().IsDraining().Return(true).Once()
	deps.lifecycleState.EXPECT().IsShutdownTimedOut().Return(false).Once()

	uc := deps.newUseCase()

	require.Error(t, uc.Readiness(ctx))
	require.NoError(t, uc.Liveness(ctx))
}
