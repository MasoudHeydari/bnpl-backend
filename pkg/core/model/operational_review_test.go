package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOperationalReviewCaseEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()

	version := int64(1)
	envelope, err := NewOperationalReviewCaseEnvelope(
		OperationalReviewEvidence{
			OperationName:  "PrepareCollectionObligation",
			SourceType:     "bnpl_contract",
			SourceID:       "contract-1",
			SourceVersion:  &version,
			IdempotencyKey: "idem-1",
			MerchantID:     "merchant-1",
			CorrelationID:  "corr-1",
		},
		"prepare_replay_unknown",
		time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)

	encoded, err := MarshalOperationalReviewCase(envelope)
	require.NoError(t, err)

	parsed, ok, err := ParseOperationalReviewCase(encoded)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, envelope.Evidence.OperationName, parsed.Evidence.OperationName)
	require.Equal(t, envelope.Evidence.IdempotencyKey, parsed.Evidence.IdempotencyKey)
}

func TestOperationalReviewFindingGuards(t *testing.T) {
	t.Parallel()

	require.True(t, OperationalReviewFindingPaymentStatusSucceeded.RequiresGetPaymentStatusConfirmation())
	require.True(t, OperationalReviewFindingNoFinancialEffectConfirmed.AllowsSafeTermination())
	require.False(t, OperationalReviewFindingFinancialEffectConfirmedOpen.AllowsSafeTermination())
	require.False(t, OperationalReviewFinding("invented").IsValid())
}
