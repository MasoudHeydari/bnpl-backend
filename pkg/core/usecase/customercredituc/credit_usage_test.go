package customercredituc

import (
	"context"
	"testing"
	"time"

	collectionsv2 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v2"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/stretchr/testify/require"
)

func TestPublicCreditUsageDoesNotExposeIndependentReasons(t *testing.T) {
	customer := model.CustomerID(uuid.New())
	ctx := context.Background()
	for _, other := range []bool{false, true} {
		t.Run(map[bool]string{false: "collections", true: "independent restriction"}[other], func(t *testing.T) {
			q := mock.NewCustomerRestrictionQueryer(t)
			rows := []model.CustomerRestriction{{RestrictionType: model.NewCreditUsageCustomerRestrictionType, Reason: model.CollectionsDefaultCustomerRestrictionReason}}
			if other {
				rows = append(rows, model.CustomerRestriction{RestrictionType: model.NewCreditUsageCustomerRestrictionType, Reason: "sensitive_internal_reason"})
			}
			q.EXPECT().ListActiveByCustomerID(ctx, customer).Return(rows, nil)
			blocked, reason, e := newCreditUsageBlocked(ctx, q, mock.NewInstallmentQueryer(t), mock.NewDefaultEpisodeQueryer(t), customer, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), collectionsv2.New(time.UTC))
			require.NoError(t, e)
			require.True(t, blocked)
			if other {
				require.Equal(t, "credit_usage_restricted", reason)
			} else {
				require.Equal(t, "collections_default", reason)
			}
		})
	}
	account := model.CreditAccount{CustomerID: customer, Status: model.ActiveCreditAccountStatus, LimitAmountMinor: 1000, ExposureAmountMinor: 1000}
	full := summaryFromAccount(account, false)
	blocked := summaryFromAccount(account, true)
	require.Zero(t, full.AvailableAmountMinor)
	require.Equal(t, "allowed", full.CreditUsage.Status)
	require.Zero(t, blocked.AvailableAmountMinor)
	require.Equal(t, "restricted", blocked.CreditUsage.Status)
	require.EqualValues(t, 1000, account.ExposureAmountMinor)
}
