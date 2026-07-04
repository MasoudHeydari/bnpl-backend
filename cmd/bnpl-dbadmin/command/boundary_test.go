package command

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminHasNoBusinessCommands(t *testing.T) {
	for _, name := range []string{"serve", "policy-assignment", "checkout", "repayment"} {
		cmd, _, e := rootCmd.Find([]string{name})
		require.True(t, e != nil || cmd == rootCmd, name)
	}
	cmd, _, e := rootCmd.Find([]string{"db", "migrate"})
	require.NoError(t, e)
	require.Equal(t, "migrate", cmd.Name())
}

func TestAdminUnknownDatabaseSubcommandFails(t *testing.T) {
	for _, args := range [][]string{{"db", "not-a-command"}, {"db", "privileges", "not-a-command"}, {"db", "ownership", "not-a-command"}, {"db", "invariants", "not-a-command"}} {
		rootCmd.SetArgs(args)
		require.Error(t, rootCmd.Execute(), args)
	}
	rootCmd.SetArgs(nil)
}
