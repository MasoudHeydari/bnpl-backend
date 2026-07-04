package shortlinkuc_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/identifier/uuidgen"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/shortlinkdestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
)

func TestCreate_RetriesAfterCodeConflictWithoutAborting(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	shortLinkRepo := mock.NewShortLink(t)
	shortLinkTxQ := mock.NewShortLinkTxQueryer(t)

	pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, conn)
		})
	conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.TxHandler) error {
			return fn(ctx, tx)
		})
	shortLinkRepo.EXPECT().Tx(tx).Return(shortLinkTxQ)

	var attempts int
	shortLinkTxQ.EXPECT().
		Create(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(
			_ context.Context,
			id model.ShortLinkID,
			code string,
			destination string,
			_ *time.Time,
			createdAt time.Time,
			updatedAt time.Time,
		) (model.ShortLink, error) {
			attempts++
			if attempts == 1 {
				return model.ShortLink{}, cerr.Conflict(nil)
			}

			return model.ShortLink{
				ID:             id,
				Code:           code,
				DestinationURL: destination,
				CreatedAt:      createdAt,
				UpdatedAt:      updatedAt,
			}, nil
		})

	destinationPolicy, err := shortlinkdestination.New([]string{"app.example"})
	require.NoError(t, err)

	uc, err := shortlinkuc.New(
		pool,
		shortLinkRepo,
		destinationPolicy,
		adapterclock.NewFixed(time.Now().UTC(), time.UTC),
		uuidgen.New(),
		metricsnoop.New(),
	)
	require.NoError(t, err)

	out, err := uc.Create(context.Background(), shortlinkuc.CreateInput{
		DestinationURL: "https://app.example/repayments",
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.False(t, out.ShortLink.ID.IsZero())
	_, err = uuid.Parse(out.ShortLink.ID.String())
	require.NoError(t, err)
	require.NotEmpty(t, out.ShortLink.Code)
}
