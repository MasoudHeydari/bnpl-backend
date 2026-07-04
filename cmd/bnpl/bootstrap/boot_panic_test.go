package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
)

func TestRecoverCommandPanic_ReportsAndConvertsToError(t *testing.T) {
	reporter := &bootstrapPanicReporter{panics: make(chan any, 1)}

	err := func() (err error) {
		defer recoverCommandPanic(
			context.Background(),
			"db_migrate",
			lognoop.New(),
			reporter,
			&err,
		)

		panic("migration-boom")
	}()

	require.Error(t, err)
	require.Contains(t, err.Error(), "db_migrate panic recovered")

	select {
	case recovered := <-reporter.panics:
		require.Equal(t, "migration-boom", recovered)

	case <-time.After(time.Second):
		t.Fatal("recovered CLI panic was not reported")
	}
}
