package checkoutsessionuc

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestUpsertWebhookDestinationInput_Validate(t *testing.T) {
	t.Parallel()

	merchantID := model.ExternalMerchantID(uuid.New())
	secret := func(n int) *string {
		value := strings.Repeat("s", n)

		return &value
	}

	cases := []struct {
		name    string
		input   UpsertWebhookDestinationInput
		wantErr bool
	}{
		{
			name:  "nil secret preserves stored secret",
			input: UpsertWebhookDestinationInput{ExternalMerchantID: merchantID, WebhookURL: "https://m.example/hook"},
		},
		{
			name: "secret at minimum length",
			input: UpsertWebhookDestinationInput{
				ExternalMerchantID: merchantID, WebhookURL: "https://m.example/hook",
				SigningSecret: secret(model.MinMerchantWebhookSigningSecretLength),
			},
		},
		{
			name: "secret below minimum",
			input: UpsertWebhookDestinationInput{
				ExternalMerchantID: merchantID, WebhookURL: "https://m.example/hook",
				SigningSecret: secret(model.MinMerchantWebhookSigningSecretLength - 1),
			},
			wantErr: true,
		},
		{
			name: "secret above maximum",
			input: UpsertWebhookDestinationInput{
				ExternalMerchantID: merchantID, WebhookURL: "https://m.example/hook",
				SigningSecret: secret(model.MaxMerchantWebhookSigningSecretLength + 1),
			},
			wantErr: true,
		},
		{
			name:    "missing url",
			input:   UpsertWebhookDestinationInput{ExternalMerchantID: merchantID},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.input.validate()
			if tc.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}
