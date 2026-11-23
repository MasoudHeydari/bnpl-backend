package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	adaptermessaging "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
)

func TestValidateConsumerSubscriptions(t *testing.T) {
	t.Parallel()

	const paymentResults = "financial.core.payment.results.v1"
	const reconciliationResults = "financial.core.installment.payment.reconciliation.results.v1"

	tests := []struct {
		name                  string
		paymentResults        string
		reconciliationResults string
		topics                []string
		wantErr               string
	}{
		{
			name:                  "exact handlers",
			paymentResults:        paymentResults,
			reconciliationResults: reconciliationResults,
			topics:                []string{paymentResults, reconciliationResults},
		},
		{
			name:                  "empty handler topic",
			paymentResults:        paymentResults,
			reconciliationResults: "",
			topics:                []string{paymentResults, reconciliationResults},
			wantErr:               `handler topic for installment_payment_reconciliation_results is empty (KAFKA_TOPIC_INSTALLMENT_PAYMENT_RECONCILIATION_RESULTS must be set)`,
		},
		{
			name:                  "empty consumer topic entry",
			paymentResults:        paymentResults,
			reconciliationResults: reconciliationResults,
			topics:                []string{paymentResults, ""},
			wantErr:               `kafka.consumer.topics contains an empty entry`,
		},
		{
			name:                  "empty consumer topics list",
			paymentResults:        paymentResults,
			reconciliationResults: reconciliationResults,
			topics:                nil,
			wantErr:               `kafka.consumer.topics is empty`,
		},
		{
			name:                  "handler consumer mismatch typo",
			paymentResults:        paymentResults,
			reconciliationResults: "inancial.core.installment.payment.reconciliation.results.v1",
			topics:                []string{paymentResults, reconciliationResults},
			wantErr:               `subscribed topic "financial.core.installment.payment.reconciliation.results.v1" has no matching handler topic`,
		},
		{
			name:                  "missing subscribed handler topic",
			paymentResults:        paymentResults,
			reconciliationResults: reconciliationResults,
			topics:                []string{paymentResults},
			wantErr:               `handler topic for installment_payment_reconciliation_results is "financial.core.installment.payment.reconciliation.results.v1" (KAFKA_TOPIC_INSTALLMENT_PAYMENT_RECONCILIATION_RESULTS) but it is not subscribed`,
		},
		{
			name:                  "extra obsolete topic",
			paymentResults:        paymentResults,
			reconciliationResults: reconciliationResults,
			topics: []string{
				paymentResults,
				"financial_core.installment_payment_sessions",
				reconciliationResults,
			},
			wantErr: `subscribed topic "financial_core.installment_payment_sessions" has no matching handler topic`,
		},
		{
			name:                  "duplicate topic",
			paymentResults:        paymentResults,
			reconciliationResults: reconciliationResults,
			topics:                []string{paymentResults, paymentResults, reconciliationResults},
			wantErr:               `duplicate subscribed topic "financial.core.payment.results.v1"`,
		},
		{
			name:                  "identical handler topics",
			paymentResults:        paymentResults,
			reconciliationResults: paymentResults,
			topics:                []string{paymentResults, reconciliationResults},
			wantErr:               `handler topics must be distinct`,
		},
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
								PaymentResults:                          tt.paymentResults,
								InstallmentPaymentReconciliationResults: tt.reconciliationResults,
							},
							Consumer: config.Consumer{Topics: tt.topics},
						},
					},
				},
			}

			err := validateConsumerSubscriptions(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
