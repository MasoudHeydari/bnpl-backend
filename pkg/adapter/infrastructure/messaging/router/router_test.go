package router_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	messagingrouter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/router"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

type recordingPublisher struct{ topics []string }

func (p *recordingPublisher) Publish(_ context.Context, topic string, _ messaging.Message) error {
	p.topics = append(p.topics, topic)

	return nil
}

func (p *recordingPublisher) Close() error { return nil }

func TestRouterSelectsPublisherByPrefix(t *testing.T) {
	t.Parallel()

	fallback, hooks := &recordingPublisher{}, &recordingPublisher{}

	r, err := messagingrouter.New(fallback, messagingrouter.Route{Prefix: "webhook://", Publisher: hooks})
	require.NoError(t, err)

	require.NoError(t, r.Publish(context.Background(), "webhook://merchant/1", messaging.Message{}))
	require.NoError(t, r.Publish(context.Background(), "merchant-checkout-events", messaging.Message{}))

	require.Equal(t, []string{"webhook://merchant/1"}, hooks.topics)
	require.Equal(t, []string{"merchant-checkout-events"}, fallback.topics)
}

func TestRouterValidatesConstruction(t *testing.T) {
	t.Parallel()

	_, err := messagingrouter.New(nil)
	require.Error(t, err)

	_, err = messagingrouter.New(&recordingPublisher{}, messagingrouter.Route{Prefix: "", Publisher: &recordingPublisher{}})
	require.Error(t, err)

	_, err = messagingrouter.New(&recordingPublisher{}, messagingrouter.Route{Prefix: "x://"})
	require.Error(t, err)
}
