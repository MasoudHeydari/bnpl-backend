package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSagasApplyDefaultsAndValidate(t *testing.T) {
	t.Parallel()

	var sagas Sagas
	sagas.ApplyDefaults()

	require.Equal(t, DefaultSagaExecutorWorkerID, sagas.Executor.WorkerID)
	require.Equal(t, DefaultSagaExecutorBatchSize, sagas.Executor.BatchSize)
	require.Equal(t, DefaultSagaExecutorLeaseDuration, sagas.Executor.LeaseDuration)
	require.Equal(t, DefaultSagaExecutorPersistenceTimeout, sagas.Executor.PersistenceTimeout)
	require.Equal(t, DefaultCheckoutConfirmationSagaRetryBase, sagas.CheckoutConfirmation.RetryBaseDelay)
	require.Equal(t, DefaultCheckoutConfirmationSagaRetryMax, sagas.CheckoutConfirmation.RetryMaxDelay)
	require.Equal(t, DefaultCheckoutConfirmationSagaMaxRetry, sagas.CheckoutConfirmation.MaxRetryAttempts)
	require.Equal(t, DefaultContractCancellationSagaRetryBase, sagas.ContractCancellation.RetryBaseDelay)
	require.Equal(t, DefaultContractCancellationSagaRetryMax, sagas.ContractCancellation.RetryMaxDelay)
	require.NoError(t, sagas.validate())
}

func TestSagasValidateRejectsNonPositivePersistenceTimeout(t *testing.T) {
	t.Parallel()

	sagas := Sagas{}
	sagas.ApplyDefaults()
	sagas.Executor.PersistenceTimeout = 0

	err := sagas.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "persistence_timeout")
}

func TestSagasValidateRejectsNonPositiveMaxRetryAttempts(t *testing.T) {
	t.Parallel()

	sagas := Sagas{}
	sagas.ApplyDefaults()
	sagas.CheckoutConfirmation.MaxRetryAttempts = 0

	err := sagas.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "max_retry_attempts")
}

func TestSagasValidateRejectsRetryMaxBelowBase(t *testing.T) {
	t.Parallel()

	sagas := Sagas{}
	sagas.ApplyDefaults()
	sagas.CheckoutConfirmation.RetryBaseDelay = 2 * time.Second
	sagas.CheckoutConfirmation.RetryMaxDelay = time.Second

	err := sagas.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "retry_max_delay")
}
