package customercredituc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	collectionsv3 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v3"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func TestNewCreditUsageBlocked_DueBeforeDefault(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	loc := time.UTC
	dueAt := time.Date(2026, 11, 1, 12, 0, 0, 0, loc)
	now := dueAt.Add(time.Hour)

	restrictionQ := mock.NewCustomerRestrictionQueryer(t)
	restrictionQ.EXPECT().
		ListActiveForCreditOwner(testifymock.Anything, customerID).
		Return(nil, nil).
		Once()

	episodeQ := mock.NewDefaultEpisodeQueryer(t)
	episodeQ.EXPECT().
		HasActiveForCreditOwner(testifymock.Anything, customerID).
		Return(false, nil).
		Once()
	episodeQ.EXPECT().
		GetActiveByCustomerID(testifymock.Anything, customerID).
		Return(model.DefaultEpisode{}, cerr.ResourceNotFound(errors.New("none"))).
		Once()

	installmentQ := mock.NewInstallmentQueryer(t)
	installmentQ.EXPECT().
		ListCollectionsObligationsForCreditOwner(testifymock.Anything, customerID).
		Return([]repo.CollectionsObligation{{
			Installment: model.Installment{
				ID:              installmentID,
				ContractID:      &contractID,
				CustomerID:      customerID,
				DueAt:           dueAt,
				GracePeriodDays: 4,
				Status:          model.DueInstallmentStatus,
			},
		}}, nil).
		Once()

	blocked, reason, err := newCreditUsageBlocked(
		ctx,
		restrictionQ,
		installmentQ,
		episodeQ,
		customerID,
		now,
		collectionsv3.New(loc),
	)
	require.NoError(t, err)
	require.True(t, blocked)
	require.Equal(t, "collections_due", reason)
}
