package customercredituc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestLimitChangeJournal(t *testing.T) {
	t.Parallel()

	t.Run("create uses limit_set absolute amount", func(t *testing.T) {
		t.Parallel()

		txType, amount := limitChangeJournal(model.CreditAccount{}, false, 100)
		require.Equal(t, model.LimitSetCreditTransactionType, txType)
		require.Equal(t, uint64(100), amount)
	})

	t.Run("increase journals positive delta", func(t *testing.T) {
		t.Parallel()

		existing := model.CreditAccount{LimitAmountMinor: 100}
		txType, amount := limitChangeJournal(existing, true, 150)
		require.Equal(t, model.LimitIncreasedCreditTransactionType, txType)
		require.Equal(t, uint64(50), amount)
	})

	t.Run("decrease journals positive delta without touching exposure", func(t *testing.T) {
		t.Parallel()

		existing := model.CreditAccount{LimitAmountMinor: 100, ExposureAmountMinor: 80}
		txType, amount := limitChangeJournal(existing, true, 60)
		require.Equal(t, model.LimitDecreasedCreditTransactionType, txType)
		require.Equal(t, uint64(40), amount)
		require.Equal(t, uint64(80), existing.ExposureAmountMinor)
	})

	t.Run("same amount refresh uses limit_set absolute amount", func(t *testing.T) {
		t.Parallel()

		existing := model.CreditAccount{LimitAmountMinor: 100}
		txType, amount := limitChangeJournal(existing, true, 100)
		require.Equal(t, model.LimitSetCreditTransactionType, txType)
		require.Equal(t, uint64(100), amount)
	})
}
