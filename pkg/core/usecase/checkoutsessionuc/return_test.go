package checkoutsessionuc

import (
	"errors"
	"net/url"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func (h *harness) expectReturnLookup(session model.CheckoutSession, sessionErr error) {
	h.sessionCQ.EXPECT().GetByID(h.ctx, h.sessionID).Return(session, sessionErr).Once()
}

func (h *harness) expectCustomerLookup() {
	h.customerCQ.EXPECT().
		GetByExternalCustomerID(h.ctx, h.profileID).
		Return(h.customerProfile(), nil).Once()
}

func TestGetReturn_CustomerOwnership(t *testing.T) {
	t.Parallel()

	otherCustomer := model.CustomerID(uuid.New())

	tests := []struct {
		name    string
		mod     func(*model.CheckoutSession)
		wantErr bool
	}{
		{
			name:    "bound customer may read",
			mod:     func(*model.CheckoutSession) {},
			wantErr: false,
		},
		{
			name:    "another customer is reported as missing",
			mod:     withCustomer(&otherCustomer),
			wantErr: true,
		},
		{
			name:    "unbound session is reported as missing",
			mod:     withCustomer(nil),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.expectPool(1, 0)

			session := h.session(withStatus(model.ProcessingCheckoutSessionStatus), tt.mod)

			h.expectReturnLookup(session, nil)
			h.expectCustomerLookup()

			output, err := h.uc.GetReturn(h.ctx, GetReturnInput{
				CheckoutSessionID: h.sessionID,
				ExternalCustomerID: h.profileID,
			})

			if tt.wantErr {
				requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
				require.Empty(t, output.ReturnURL)

				return
			}

			require.NoError(t, err)
			require.Equal(t, h.sessionID, output.Session.ID)
		})
	}
}

func TestGetReturn_MissingSessionIsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	h.expectReturnLookup(model.CheckoutSession{}, cerr.ResourceNotFound(errors.New("no such session")))

	_, err := h.uc.GetReturn(h.ctx, GetReturnInput{
		CheckoutSessionID: h.sessionID,
		ExternalCustomerID: h.profileID,
	})
	requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
}

func TestGetReturn_RequiresIdentifiers(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.uc.GetReturn(h.ctx, GetReturnInput{ExternalCustomerID: h.profileID})
	require.Error(t, err)

	_, err = h.uc.GetReturn(h.ctx, GetReturnInput{CheckoutSessionID: h.sessionID})
	require.Error(t, err)
}

func TestGetReturn_CompletedReturnURLCarriesOnlyCheckoutIdentityAndState(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	completed := h.session(withStatus(model.CompletedCheckoutSessionStatus), withContract(h.contractID))
	h.expectReturnLookup(completed, nil)
	h.expectCustomerLookup()

	output, err := h.uc.GetReturn(h.ctx, GetReturnInput{
		CheckoutSessionID: h.sessionID,
		ExternalCustomerID: h.profileID,
	})
	require.NoError(t, err)
	require.Equal(t, model.CompletedCheckoutSessionStatus, output.Session.Status)

	parsed, err := url.Parse(output.ReturnURL)
	require.NoError(t, err)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "shop.example", parsed.Host)
	require.Equal(t, "/return", parsed.Path)

	keys := make([]string, 0, len(parsed.Query()))
	for key := range parsed.Query() {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	// Browser return is UX only: no payment truth, contract, or status leaks.
	require.Equal(t, []string{"checkout_session_id", "state"}, keys)
	require.Equal(t, h.publicToken.String(), parsed.Query().Get("checkout_session_id"))
	require.Equal(t, "cart-42", parsed.Query().Get("state"))
	require.NotContains(t, output.ReturnURL, h.contractID.String())
	require.NotContains(t, output.ReturnURL, "completed")
}
