package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsumerSubscriptionTopicsDoesNotMutateConfiguration(t *testing.T) {
	consumer := Consumer{
		Topics:                     []string{"payment-results", "reconciliation-results"},
		MerchantParticipationTopic: MerchantParticipationTopic,
	}

	topics := consumer.SubscriptionTopics()
	require.Equal(t, []string{"payment-results", "reconciliation-results", MerchantParticipationTopic}, topics)
	topics[0] = "changed"
	require.Equal(t, []string{"payment-results", "reconciliation-results"}, consumer.Topics)

	consumer.MerchantParticipationTopic = ""
	require.Equal(t, consumer.Topics, consumer.SubscriptionTopics())
}
