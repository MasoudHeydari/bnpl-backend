package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRepos(t *testing.T) {
	t.Parallel()

	repos := NewRepos()

	require.NotNil(t, repos.Migrations)
	require.NotNil(t, repos.Outbox)
	require.NotNil(t, repos.Health)

	var (
		_ = repos.Migrations
		_ = repos.Outbox
		_ = repos.Health
	)
}
