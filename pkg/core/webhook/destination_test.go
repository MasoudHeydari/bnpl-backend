package webhook_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/webhook"
)

func TestParseMerchantDestinationRoundTrip(t *testing.T) {
	t.Parallel()

	merchantID, err := model.ParseMerchantID("5f6c1f02-0f0e-4c7a-9a83-0d4d4d2f1b11")
	require.NoError(t, err)

	destination := webhook.MerchantDestination(merchantID)
	require.True(t, webhook.IsDestination(destination))

	parsed, err := webhook.ParseMerchantDestination(destination)
	require.NoError(t, err)
	require.Equal(t, merchantID, parsed)

	_, err = webhook.ParseMerchantDestination("kafka.topic")
	require.Error(t, err)

	_, err = webhook.ParseMerchantDestination(webhook.DestinationPrefix + "not-a-uuid")
	require.Error(t, err)
}
