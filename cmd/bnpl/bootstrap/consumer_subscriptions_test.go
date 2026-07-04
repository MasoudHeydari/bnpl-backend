package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	adaptermessaging "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
)

func TestValidateConsumerSubscriptions(t *testing.T) {
	t.Parallel()

	const paymentResults = "financial-core.payment-results"
	const reconciliationResults = "financial_core.installment_payment_reconciliation_results"

	tests := []struct {
		name   string
		topics []string
		valid  bool
	}{
		{name: "exact handlers", topics: []string{paymentResults, reconciliationResults}, valid: true},
		{name: "extra obsolete topic", topics: []string{paymentResults, "financial_core.installment_payment_sessions", reconciliationResults}},
		{name: "missing reconciliation", topics: []string{paymentResults}},
		{name: "duplicate topic", topics: []string{paymentResults, paymentResults, reconciliationResults}},
		{name: "no topics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Config{
				Infrastructure: config.Infrastructure{
					Messaging: config.Messaging{
						Provider: adaptermessaging.KafkaProvider,
						Kafka: config.Kafka{
							Topics: config.Topics{
								PaymentResults:                           paymentResults,
								InstallmentPaymentReconciliationResults: reconciliationResults,
							},
							Consumer: config.Consumer{Topics: tt.topics},
						},
					},
				},
			}

			err := validateConsumerSubscriptions(cfg)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
