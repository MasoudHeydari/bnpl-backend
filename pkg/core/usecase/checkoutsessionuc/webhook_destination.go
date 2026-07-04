package checkoutsessionuc

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UpsertWebhookDestination registers or updates the merchant's single lifecycle
// webhook endpoint.
//
// Checkout requests never supply a webhook URL. Lifecycle events resolve the
// destination from this table at enqueue time. The signing secret is write-only:
// a nil SigningSecret preserves the stored ciphertext, a non-nil value rotates
// it, and the first registration must supply one.
func (uc *UseCase) UpsertWebhookDestination(
	ctx context.Context,
	input UpsertWebhookDestinationInput,
) (UpsertWebhookDestinationOutput, error) {
	const op = "checkoutsessionuc.UseCase.UpsertWebhookDestination"

	if err := input.validate(); err != nil {
		return UpsertWebhookDestinationOutput{}, err
	}

	var (
		rotatedCiphertext string
		rotatedKeyID      string
	)

	if input.SigningSecret != nil {
		ciphertext, keyID, encryptErr := uc.webhookSecretBox.Encrypt(*input.SigningSecret)
		if encryptErr != nil {
			return UpsertWebhookDestinationOutput{}, fmt.Errorf("%s: encrypt signing secret: %w", op, encryptErr)
		}

		rotatedCiphertext = ciphertext
		rotatedKeyID = keyID
	}

	var destination model.MerchantWebhookDestination

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					merchantProfileQ := uc.merchantProfile.Tx(tx)
					sessionQ := uc.checkoutSession.Tx(tx)

					merchant, merchantErr := merchantProfileQ.GetByMerchantID(ctx, input.MerchantID)
					if merchantErr != nil {
						return merchantErr
					}

					existing, findErr := sessionQ.FindWebhookDestination(ctx, merchant.MerchantID)
					if findErr != nil && !cerr.IsCode(findErr, cerr.CodeResourceNotFound) {
						return findErr
					}

					exists := findErr == nil

					ciphertext := rotatedCiphertext
					keyID := rotatedKeyID

					if input.SigningSecret == nil {
						if !exists || existing.SigningSecretCiphertext == "" {
							return webhookSigningSecretRequiredError()
						}

						ciphertext = existing.SigningSecretCiphertext
						keyID = existing.SigningSecretKeyID
					}

					enabled := true
					if input.Enabled != nil {
						enabled = *input.Enabled
					} else if exists {
						enabled = existing.Enabled
					}

					now := uc.clock.Now().UTC()

					saved, saveErr := sessionQ.UpsertWebhookDestination(
						ctx,
						merchant.MerchantID,
						input.WebhookURL,
						ciphertext,
						keyID,
						enabled,
						now,
					)
					if saveErr != nil {
						return saveErr
					}

					destination = saved

					return nil
				},
			)
		},
	)
	if err != nil {
		return UpsertWebhookDestinationOutput{}, err
	}

	return UpsertWebhookDestinationOutput{Destination: destination}, nil
}

func (i UpsertWebhookDestinationInput) validate() error {
	if i.MerchantID.IsZero() {
		return merchantIDRequiredError()
	}

	if i.SigningSecret != nil {
		if len(*i.SigningSecret) < model.MinMerchantWebhookSigningSecretLength {
			return webhookSigningSecretTooShortError()
		}

		if len(*i.SigningSecret) > model.MaxMerchantWebhookSigningSecretLength {
			return webhookSigningSecretTooLongError()
		}
	}

	return validateAbsoluteHTTPURL(i.WebhookURL, "webhook_url")
}

func validateAbsoluteHTTPURL(raw string, field string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		switch field {
		case "return_url":
			return validationError("REQUIRED", field, "error.return_url_required")

		case "webhook_url":
			return validationError("REQUIRED", field, "error.webhook_url_required")

		default:
			return validationError("REQUIRED", field, "error.return_url_required")
		}
	}

	if len(trimmed) > 2048 {
		switch field {
		case "return_url":
			return validationError("MAX_LENGTH", field, "error.return_url_too_long")

		case "webhook_url":
			return validationError("MAX_LENGTH", field, "error.webhook_url_too_long")

		default:
			return validationError("MAX_LENGTH", field, "error.return_url_too_long")
		}
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil || parsed.Opaque != "" || parsed.Hostname() == "" ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") ||
		parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" ||
		strings.ContainsAny(trimmed, "\r\n\t") {
		switch field {
		case "return_url":
			return validationError("INVALID", field, "error.return_url_invalid")

		case "webhook_url":
			return validationError("INVALID", field, "error.webhook_url_invalid")

		default:
			return validationError("INVALID", field, "error.return_url_invalid")
		}
	}

	return nil
}
