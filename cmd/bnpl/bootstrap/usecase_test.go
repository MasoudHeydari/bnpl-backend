package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewUseCases_NoopConfig(t *testing.T) {
	t.Parallel()

	app := testAppWithInfrastructure(t, noopTestConfig())

	usecases, err := app.NewUseCases()
	require.NoError(t, err)

	require.NotNil(t, usecases.Migrations)
	require.NotNil(t, usecases.OutboxPublisher)
	require.NotNil(t, usecases.Health)
}
