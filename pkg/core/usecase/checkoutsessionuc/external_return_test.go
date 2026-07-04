package checkoutsessionuc

import (
	"errors"
	"net/url"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func (h *harness) expectExternalReturnLookup(session model.CheckoutSession, sessionErr error) {
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, sessionErr).Once()
}

func TestReturnExternal_CompletedRedirectsToRegisteredMerchantURL(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	completed := h.session(withStatus(model.CompletedCheckoutSessionStatus))
	h.expectExternalReturnLookup(completed, nil)

	output, err := h.uc.ReturnExternal(h.ctx, ReturnExternalInput{Token: h.publicToken})
	require.NoError(t, err)

	parsed, err := url.Parse(output.RedirectURL)
	require.NoError(t, err)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "shop.example", parsed.Host)
	require.Equal(t, "/return", parsed.Path)

	keys := make([]string, 0, len(parsed.Query()))
	for key := range parsed.Query() {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	require.Equal(t, []string{"checkout_session_id", "state"}, keys)
	require.Equal(t, h.publicToken.String(), parsed.Query().Get("checkout_session_id"))
	require.Equal(t, "cart-42", parsed.Query().Get("state"))
}

func TestReturnExternal_OmitsStateQueryWhenMerchantStateEmpty(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	completed := h.session(
		withStatus(model.CompletedCheckoutSessionStatus),
		func(s *model.CheckoutSession) { s.MerchantState = "" },
	)
	h.expectExternalReturnLookup(completed, nil)

	output, err := h.uc.ReturnExternal(h.ctx, ReturnExternalInput{Token: h.publicToken})
	require.NoError(t, err)

	parsed, err := url.Parse(output.RedirectURL)
	require.NoError(t, err)
	require.Equal(t, []string{"checkout_session_id"}, sortedQueryKeys(parsed))
	require.Equal(t, h.publicToken.String(), parsed.Query().Get("checkout_session_id"))
	require.Empty(t, parsed.Query().Get("state"))
}

func TestReturnExternal_UnknownTokenIsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	h.expectExternalReturnLookup(
		model.CheckoutSession{},
		cerr.ResourceNotFound(errors.New("no such token")),
	)

	_, err := h.uc.ReturnExternal(h.ctx, ReturnExternalInput{Token: h.publicToken})
	requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
}

func TestReturnExternal_InternalOriginIsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	internalSession := h.session(withStatus(model.CompletedCheckoutSessionStatus))
	internalSession.Origin = model.InternalCheckoutSessionOrigin
	h.expectExternalReturnLookup(internalSession, nil)

	_, err := h.uc.ReturnExternal(h.ctx, ReturnExternalInput{Token: h.publicToken})
	requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
}

func sortedQueryKeys(parsed *url.URL) []string {
	keys := make([]string, 0, len(parsed.Query()))
	for key := range parsed.Query() {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}
