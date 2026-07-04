package command

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeHasNoDatabaseAdministrationCommands(t *testing.T) {
	for _, name := range []string{"db", "migrate", "init-dev", "init-prod", "ownership", "privileges"} {
		cmd, _, e := rootCmd.Find([]string{name})
		require.True(t, e != nil || cmd == rootCmd, name)
	}
	cmd, _, e := rootCmd.Find([]string{"policy-assignment", "import-trusted"})
	require.NoError(t, e)
	require.Equal(t, "import-trusted", cmd.Name())
}
