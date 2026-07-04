package noop_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/external/merchantwebhook/noop"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

func TestPublisher_SkipsWithoutHTTPAndAcknowledges(t *testing.T) {
	t.Parallel()

	publisher, err := noop.NewPublisher(lognoop.New(), metricsnoop.New())
	require.NoError(t, err)

	err = publisher.Publish(context.Background(), "webhook://merchant/5f6c1f02-0f0e-4c7a-9a83-0d4d4d2f1b11", messaging.Message{
		Headers: map[string][]byte{"event_type": []byte("bnpl.checkout.completed.v1")},
	})
	require.Error(t, err)
	require.True(t, messaging.IsSkipped(err))
	require.False(t, messaging.IsNonRetryable(err))
	require.NoError(t, publisher.Close())
}

func TestPublisher_RejectsNonWebhookDestination(t *testing.T) {
	t.Parallel()

	publisher, err := noop.NewPublisher(lognoop.New(), metricsnoop.New())
	require.NoError(t, err)

	err = publisher.Publish(context.Background(), "kafka.topic", messaging.Message{})
	require.Error(t, err)
	require.True(t, messaging.IsNonRetryable(err))
}

func TestNewPublisher_RequiresDependencies(t *testing.T) {
	t.Parallel()

	_, err := noop.NewPublisher(nil, metricsnoop.New())
	require.Error(t, err)

	_, err = noop.NewPublisher(lognoop.New(), nil)
	require.Error(t, err)
}
