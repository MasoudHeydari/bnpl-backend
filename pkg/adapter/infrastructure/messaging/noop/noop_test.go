package noop_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterhealth "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/health"
	messagingnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/noop"
	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

func TestPublisherSucceedsWithoutBroker(t *testing.T) {
	publisher := messagingnoop.NewPublisher()

	err := publisher.Publish(
		context.Background(),
		"payments",
		messaging.Message{Key: []byte("1"), Value: []byte(`{"ok":true}`)},
	)
	require.NoError(t, err)
	require.NoError(t, publisher.Close())
}

func TestPublisherRespectsContextCancellation(t *testing.T) {
	publisher := messagingnoop.NewPublisher()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := publisher.Publish(ctx, "payments", messaging.Message{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestSubscriberBlocksUntilContextCanceled(t *testing.T) {
	subscriber := messagingnoop.NewSubscriber()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := subscriber.Subscribe(ctx, func(context.Context, messaging.Message) error {
		t.Fatal("noop subscriber must not invoke handler")
		return nil
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, subscriber.Close())
}

func TestSubscriberReportsHealthyUntilCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	state := adapterhealth.NewConsumerState()
	subscriber := messagingnoop.NewSubscriberWithHealth(state)
	done := make(chan error, 1)
	go func() {
		done <- subscriber.Subscribe(ctx, func(context.Context, messaging.Message) error { return nil })
	}()
	require.Eventually(t, state.IsReady, time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	assert.Equal(t, corehealth.ConsumerStopped, state.State())
}
