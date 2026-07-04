package financialcorepaymentresultuc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

func TestValidateMinimumTrustedEnvelope_RejectsNonFinancialCoreSource(t *testing.T) {
	t.Parallel()

	input := ProcessInput{
		EventID:       "event-1",
		EventType:     expectedPaymentResultEventType,
		EventVersion:  expectedPaymentResultEventVersion,
		SourceService: "bank",
		RawPayload:    json.RawMessage(`{"event_id":"event-1"}`),
	}

	err := validateMinimumTrustedEnvelope(input)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestValidateMinimumTrustedEnvelope_AcceptsFinancialCoreSource(t *testing.T) {
	t.Parallel()

	input := ProcessInput{
		EventID:       "event-1",
		EventType:     expectedPaymentResultEventType,
		EventVersion:  expectedPaymentResultEventVersion,
		SourceService: expectedPaymentResultSourceService,
		RawPayload:    json.RawMessage(`{"event_id":"event-1"}`),
	}

	require.NoError(t, validateMinimumTrustedEnvelope(input))
}

func TestValidateMinimumTrustedEnvelope_RejectsUnsupportedEventType(t *testing.T) {
	t.Parallel()

	input := ProcessInput{
		EventID:       "event-1",
		EventType:     "financial_core.some_other_event.v1",
		EventVersion:  expectedPaymentResultEventVersion,
		SourceService: expectedPaymentResultSourceService,
		RawPayload:    json.RawMessage(`{"event_id":"event-1"}`),
	}

	err := validateMinimumTrustedEnvelope(input)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestValidateMinimumTrustedEnvelope_RejectsUnsupportedEventVersion(t *testing.T) {
	t.Parallel()

	input := ProcessInput{
		EventID:       "event-1",
		EventType:     expectedPaymentResultEventType,
		EventVersion:  "2",
		SourceService: expectedPaymentResultSourceService,
		RawPayload:    json.RawMessage(`{"event_id":"event-1"}`),
	}

	err := validateMinimumTrustedEnvelope(input)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}
