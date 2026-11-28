//go:build development && integration

package merchantparticipationrp_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/inboxrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/merchantparticipationrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/merchantprofilerp"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/identifier/uuidgen"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	merchantprovisioningv1 "github.com/parstis/bnpl-backend/pkg/core/policy/merchantprovisioning/v1"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantprovisioninguc"
	"github.com/stretchr/testify/require"
)

type merchantClient struct {
	reference   merchant.Reference
	readCount   int
	unavailable bool
	reports     []merchant.Progress
}

func (f *merchantClient) GetParticipation(context.Context, model.MerchantParticipationRequestID) (merchant.Reference, error) {
	f.readCount++
	if f.unavailable {
		return merchant.Reference{}, errors.New("synthetic outage")
	}

	return f.reference, nil
}

func (f *merchantClient) ReportReady(_ context.Context, p merchant.Progress) (merchant.Snapshot, error) {
	f.reports = append(f.reports, p)
	if f.unavailable {
		return merchant.Snapshot{}, errors.New("synthetic timeout")
	}

	return merchant.Snapshot{
		RequestID:        p.RequestID,
		BusinessLine:     "bnpl",
		Environment:      "SANDBOX",
		Status:           "READY",
		OwnerVersion:     p.OwnerVersion,
		AccountReference: p.AccountReference,
	}, nil
}

func TestProvisionReplayAndDurableCallback(t *testing.T) {
	dsn := os.Getenv("COLLECTIONS_TEST_DATABASE_URL")
	require.NotEmpty(t, dsn, "dedicated disposable PostgreSQL is required")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	schema := "merchant_participation_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := postgres.NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Pool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer pool.Pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	ddl, err := os.ReadFile("../migrationsrp/000_schema.sql")
	require.NoError(t, err)
	baseline := strings.SplitN(string(ddl), "-- MANAGED DATABASE PERMISSIONS: canonical initial grants (pre-release)", 2)[0]
	baseline = strings.ReplaceAll(baseline, "SET search_path TO bnpl;", "SET search_path TO "+schema+";")
	baseline = strings.ReplaceAll(baseline, "bnpl.", schema+".")
	baseline = strings.ReplaceAll(baseline, "SET search_path = bnpl, pg_temp", "SET search_path = "+schema+", pg_temp")
	baseline = strings.ReplaceAll(baseline, "pg_catalog, bnpl", "pg_catalog, "+schema)
	_, err = pool.Pool.Exec(ctx, baseline)
	require.NoError(t, err)
	mid := model.ExternalMerchantID(uuid.New())
	rid := model.MerchantParticipationRequestID(uuid.New())
	client := &merchantClient{reference: merchant.Reference{
		MerchantID:            mid,
		RequestID:             rid,
		BusinessLine:          "bnpl",
		Environment:           "SANDBOX",
		TradeName:             "Synthetic store",
		BaseApplicationStatus: "APPROVED",
		SelfDeclaration: &model.MerchantSelfDeclaration{
			MaxAmountMinor:    370_000_000,
			MaxAmountCurrency: model.IRRCurrency,
			MaxTenureMonths:   7,
		},
	}}
	clk := adapterclock.NewFixed(time.Date(
		2026,
		11,
		28,
		8,
		30,
		0,
		0,
		time.UTC,
	), time.UTC)
	makeUC := func() *merchantprovisioninguc.UseCase {
		uc, e := merchantprovisioninguc.New(
			pool,
			inboxrp.New(),
			merchantparticipationrp.New(),
			merchantprofilerp.New(),
			client,
			&clk,
			uuidgen.New(),
			lognoop.New(),
			merchantprovisioningv1.New(),
		)
		require.NoError(t, e)
		return uc
	}
	input := merchantprovisioninguc.Input{
		MerchantID:    mid,
		RequestID:     rid,
		ApplicationID: model.MerchantApplicationID(uuid.New()),
		Environment:   "SANDBOX",
		Event: model.InboxEvent{
			EventID:            uuid.NewString(),
			EventType:          "merchant_product_participation_requested",
			EventVersion:       "1",
			SourceService:      "merchant_service",
			Payload:            []byte(`{}`),
			PayloadFingerprint: "sha256:" + strings.Repeat("a", 64),
			ConsumerName:       "merchant_participation",
		},
	}
	uc := makeUC()
	declaration := client.reference.SelfDeclaration
	client.reference.SelfDeclaration = nil
	require.ErrorIs(t, uc.Process(ctx, input), merchantprovisioninguc.ErrInvalidReference)
	var accountCount, requestCount int
	err = pool.Pool.QueryRow(ctx, `
SELECT
    (SELECT count(*) FROM merchant_profiles),
    (SELECT count(*) FROM merchant_participation_requests)
`).Scan(&accountCount, &requestCount)
	require.NoError(t, err)
	require.Zero(t, accountCount)
	require.Zero(t, requestCount)
	invalidDeclaration := *declaration
	invalidDeclaration.MaxTenureMonths = 0
	client.reference.SelfDeclaration = &invalidDeclaration
	require.ErrorIs(t, uc.Process(ctx, input), merchantprovisioninguc.ErrInvalidReference)
	// Neither platform ceiling may be exceeded, even if the other limit is valid.
	for _, outsidePolicy := range []model.MerchantSelfDeclaration{
		{
			MaxAmountMinor:    5_000_000_001,
			MaxAmountCurrency: model.IRRCurrency,
			MaxTenureMonths:   7,
		},
		{
			MaxAmountMinor:    370_000_000,
			MaxAmountCurrency: model.IRRCurrency,
			MaxTenureMonths:   13,
		},
	} {
		client.reference.SelfDeclaration = &outsidePolicy
		require.ErrorIs(t, uc.Process(ctx, input), merchantprovisioninguc.ErrInvalidReference)
	}

	err = pool.Pool.QueryRow(ctx, `
SELECT
    (SELECT count(*) FROM merchant_profiles),
    (SELECT count(*) FROM merchant_participation_requests)
`).Scan(&accountCount, &requestCount)
	require.NoError(t, err)
	require.Zero(t, accountCount)
	require.Zero(t, requestCount)
	client.reference.SelfDeclaration = declaration
	require.NoError(t, uc.Process(ctx, input))
	client.unavailable = true
	require.NoError(t, uc.Process(ctx, input))
	changed := input
	changed.Event.PayloadFingerprint = "sha256:" + strings.Repeat("b", 64)
	require.ErrorIs(t, uc.Process(ctx, changed), merchantprovisioninguc.ErrConflict)
	// A distinct event for the same command must not need another HTTP read.
	replay := input
	replay.Event.EventID = uuid.NewString()
	require.NoError(t, uc.Process(ctx, replay))
	require.Equal(t, 5, client.readCount)
	var accounts, requests, terminals int
	err = pool.Pool.QueryRow(ctx, `
SELECT
    (SELECT count(*) FROM merchant_profiles),
    (SELECT count(*) FROM merchant_participation_requests),
    (SELECT count(*) FROM merchant_terminals)
`).Scan(&accounts, &requests, &terminals)
	require.NoError(t, err)
	require.Equal(t, 1, accounts)
	require.Equal(t, 1, requests)
	require.Zero(t, terminals)
	var maxAmountMinor int64
	var maxTenureMonths int
	err = pool.Pool.QueryRow(ctx, `
SELECT max_amount_minor, max_tenure_months
FROM merchant_profiles
WHERE external_merchant_id = $1
`, mid).Scan(&maxAmountMinor, &maxTenureMonths)
	require.NoError(t, err)
	require.Equal(t, int64(370_000_000), maxAmountMinor)
	require.Equal(t, 7, maxTenureMonths)
	require.NoError(t, uc.DeliverDue(ctx))
	require.Len(t, client.reports, 1)
	// Construct a new use case to simulate a process restart and retry the same immutable result.
	clk.Time = clk.Time.Add(time.Minute)
	client.unavailable = false
	require.NoError(t, makeUC().DeliverDue(ctx))
	require.Len(t, client.reports, 2)
	require.Equal(t, client.reports[0], client.reports[1])
	var status string
	err = pool.Pool.QueryRow(ctx, `SELECT delivery_status FROM merchant_participation_requests WHERE request_id=$1`, rid).Scan(&status)
	require.NoError(t, err)
	require.Equal(t, "delivered", status)

	// A new request must not silently overwrite an existing account's limits.
	newRequest := input
	newRequest.RequestID = model.MerchantParticipationRequestID(uuid.New())
	newRequest.Event.EventID = uuid.NewString()
	client.reference.RequestID = newRequest.RequestID
	differentDeclaration := *declaration
	differentDeclaration.MaxAmountMinor++
	client.reference.SelfDeclaration = &differentDeclaration
	require.ErrorIs(t, uc.Process(ctx, newRequest), merchantprovisioninguc.ErrConflict)
	err = pool.Pool.QueryRow(ctx, `SELECT max_amount_minor FROM merchant_profiles WHERE external_merchant_id=$1`, mid).Scan(&maxAmountMinor)
	require.NoError(t, err)
	require.Equal(t, int64(370_000_000), maxAmountMinor)
}
