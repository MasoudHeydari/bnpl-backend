package serdser

import (
	"context"

	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
	i18nbuiltin "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n/builtin"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

var testLocalizer = func() resti18n.Localizer {
	localizer, err := i18nbuiltin.New(resti18n.LocaleENUS)
	if err != nil {
		panic(err)
	}

	return localizer
}()

func withTestLocalization(ctx context.Context, locale resti18n.Locale) context.Context {
	return resti18n.WithRequestState(ctx, locale, testLocalizer)
}

func localizedMessageForTest(err *cerr.Error) string {
	message, localizeErr := testLocalizer.Localize(
		resti18n.LocaleENUS,
		string(err.MessageKey),
		resti18n.Params(err.MessageParams),
	)
	if localizeErr != nil {
		panic(localizeErr)
	}

	return message
}
