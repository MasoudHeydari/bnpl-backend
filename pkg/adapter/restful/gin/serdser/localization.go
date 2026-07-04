package serdser

import (
	"context"

	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

const hardFallbackErrorMessage = "An unexpected error occurred."
const hardFallbackSuccessMessage = "Request completed successfully."

func localizeError(ctx context.Context, key string, params resti18n.Params) string {
	if message, err := resti18n.Localize(ctx, key, params); err == nil && message != "" {
		return message
	}

	fallbackKey := string(cerr.DefaultMessageKey(cerr.CodeInternalError))
	if key != fallbackKey {
		if message, err := resti18n.Localize(ctx, fallbackKey, nil); err == nil && message != "" {
			return message
		}
	}

	return hardFallbackErrorMessage
}

func localizeSuccess(ctx context.Context, key SuccessMessageKey) string {
	if message, err := resti18n.Localize(ctx, key.String(), nil); err == nil && message != "" {
		return message
	}

	if message, err := resti18n.Localize(ctx, SuccessMessageDefault.String(), nil); err == nil && message != "" {
		return message
	}

	return hardFallbackSuccessMessage
}
