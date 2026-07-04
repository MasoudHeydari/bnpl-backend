package merchantterminaluc

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	fingerprinthmac "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/hmac"
	adaptersecretbox "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/secretbox"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// The fakes below are in-memory queryer implementations returned by generated
// Core repo mocks from Conn/Tx. UseCase collaborators are always mockery mocks.

// fakeMerchantProfiles serves merchant profiles by merchant id.
type fakeMerchantProfiles struct {
	profiles map[model.MerchantID]model.MerchantProfile
}

func (f *fakeMerchantProfiles) Conn(repo.Conn) repo.MerchantProfileConnQueryer {
	panic("merchant profile Conn is not used by this use case")
}

func (f *fakeMerchantProfiles) Tx(repo.Tx) repo.MerchantProfileTxQueryer {
	return fakeMerchantProfileTx{profiles: f.profiles}
}

type fakeMerchantProfileTx struct {
	repo.MerchantProfileTxQueryer
	profiles map[model.MerchantID]model.MerchantProfile
}

func (f fakeMerchantProfileTx) GetByMerchantID(
	_ context.Context,
	merchantID model.MerchantID,
) (model.MerchantProfile, error) {
	profile, ok := f.profiles[merchantID]
	if !ok {
		return model.MerchantProfile{}, cerr.ResourceNotFound(fmt.Errorf("merchant profile not found"))
	}

	return profile, nil
}

// fakeTerminals is an in-memory repo.MerchantTerminal. Conn and Tx share state;
// the fake does not roll back, which is acceptable because the use case never
// writes before it can still fail on a path the tests inspect.
type fakeTerminals struct {
	terminals   []model.MerchantTerminal
	credentials []model.MerchantTerminalCredential
	deliveries  []model.MerchantTerminalIssuanceDelivery
	// permissions maps merchant_terminal_id -> assigned wire permission codes.
	permissions map[model.MerchantTerminalID][]string

	// terminalIDTakenFailures makes the next N CreateTerminal calls report a
	// terminal id collision.
	terminalIDTakenFailures int
	createTerminalCalls     int
	writes                  int
}

var _ repo.MerchantTerminal = (*fakeTerminals)(nil)

func (f *fakeTerminals) Conn(repo.Conn) repo.MerchantTerminalConnQueryer { return f }

func (f *fakeTerminals) Tx(repo.Tx) repo.MerchantTerminalTxQueryer { return f }

func notFound() error { return cerr.ResourceNotFound(fmt.Errorf("not found")) }

func (f *fakeTerminals) GetByTerminalID(
	_ context.Context,
	terminalID model.TerminalID,
) (model.MerchantTerminal, error) {
	for _, terminal := range f.terminals {
		if terminal.TerminalID == terminalID {
			return terminal, nil
		}
	}

	return model.MerchantTerminal{}, notFound()
}

func (f *fakeTerminals) GetByMerchantID(
	_ context.Context,
	merchantID model.MerchantID,
) (model.MerchantTerminal, error) {
	for _, terminal := range f.terminals {
		if terminal.MerchantID == merchantID {
			return terminal, nil
		}
	}

	return model.MerchantTerminal{}, notFound()
}

func (f *fakeTerminals) LockByTerminalID(
	ctx context.Context,
	terminalID model.TerminalID,
) (model.MerchantTerminal, error) {
	return f.GetByTerminalID(ctx, terminalID)
}

func (f *fakeTerminals) LockByMerchantID(
	ctx context.Context,
	merchantID model.MerchantID,
) (model.MerchantTerminal, error) {
	return f.GetByMerchantID(ctx, merchantID)
}

func (f *fakeTerminals) CreateTerminal(
	_ context.Context,
	merchantID model.MerchantID,
	terminalID model.TerminalID,
	status model.MerchantTerminalStatus,
	createdAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminal, error) {
	f.createTerminalCalls++

	if f.terminalIDTakenFailures > 0 {
		f.terminalIDTakenFailures--

		return model.MerchantTerminal{}, cerr.Conflict(repo.ErrMerchantTerminalIDTaken)
	}

	for _, terminal := range f.terminals {
		if terminal.MerchantID == merchantID {
			return model.MerchantTerminal{}, cerr.Conflict(repo.ErrMerchantTerminalExists)
		}
	}

	f.writes++

	terminal := model.MerchantTerminal{
		ID:         model.MerchantTerminalID(uuid.New()),
		MerchantID: merchantID,
		TerminalID: terminalID,
		Status:     status,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}
	f.terminals = append(f.terminals, terminal)

	return terminal, nil
}

func (f *fakeTerminals) GetActiveCredential(
	_ context.Context,
	merchantTerminalID model.MerchantTerminalID,
) (model.MerchantTerminalCredential, error) {
	for _, credential := range f.credentials {
		if credential.MerchantTerminalID == merchantTerminalID &&
			credential.Status == model.ActiveMerchantTerminalCredentialStatus {
			return credential, nil
		}
	}

	return model.MerchantTerminalCredential{}, notFound()
}

func (f *fakeTerminals) ListAcceptableCredentials(
	_ context.Context,
	merchantTerminalID model.MerchantTerminalID,
	now time.Time,
) ([]model.MerchantTerminalCredential, error) {
	var out []model.MerchantTerminalCredential

	for _, credential := range f.credentials {
		if credential.MerchantTerminalID == merchantTerminalID && credential.IsAcceptableAt(now) {
			out = append(out, credential)
		}
	}

	return out, nil
}

func (f *fakeTerminals) GetCredentialByID(
	_ context.Context,
	id model.MerchantTerminalCredentialID,
) (model.MerchantTerminalCredential, error) {
	for _, credential := range f.credentials {
		if credential.ID == id {
			return credential, nil
		}
	}

	return model.MerchantTerminalCredential{}, notFound()
}

func (f *fakeTerminals) LockCredentialsForRotation(
	_ context.Context,
	merchantTerminalID model.MerchantTerminalID,
) ([]model.MerchantTerminalCredential, error) {
	var out []model.MerchantTerminalCredential

	for _, credential := range f.credentials {
		if credential.MerchantTerminalID != merchantTerminalID {
			continue
		}

		if credential.Status == model.ActiveMerchantTerminalCredentialStatus ||
			credential.Status == model.RetiringMerchantTerminalCredentialStatus {
			out = append(out, credential)
		}
	}

	slices.SortFunc(out, func(a, b model.MerchantTerminalCredential) int {
		return int(a.Version - b.Version)
	})

	return out, nil
}

func (f *fakeTerminals) CreateCredential(
	_ context.Context,
	merchantTerminalID model.MerchantTerminalID,
	version int64,
	apiKeyHash string,
	status model.MerchantTerminalCredentialStatus,
	activatedAt time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	f.writes++

	credential := model.MerchantTerminalCredential{
		ID:                 model.MerchantTerminalCredentialID(uuid.New()),
		MerchantTerminalID: merchantTerminalID,
		Version:            version,
		APIKeyHash:         apiKeyHash,
		Status:             status,
		CreatedAt:          createdAt,
		ActivatedAt:        activatedAt,
		UpdatedAt:          updatedAt,
	}
	f.credentials = append(f.credentials, credential)

	return credential, nil
}

func (f *fakeTerminals) update(
	id model.MerchantTerminalCredentialID,
	mutate func(*model.MerchantTerminalCredential),
) (model.MerchantTerminalCredential, error) {
	for index := range f.credentials {
		if f.credentials[index].ID == id {
			f.writes++

			mutate(&f.credentials[index])

			return f.credentials[index], nil
		}
	}

	return model.MerchantTerminalCredential{}, notFound()
}

func (f *fakeTerminals) MarkRetiring(
	_ context.Context,
	id model.MerchantTerminalCredentialID,
	retireAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	return f.update(id, func(c *model.MerchantTerminalCredential) {
		c.Status = model.RetiringMerchantTerminalCredentialStatus
		c.RetireAt = &retireAt
		c.UpdatedAt = updatedAt
	})
}

func (f *fakeTerminals) MarkRetired(
	_ context.Context,
	id model.MerchantTerminalCredentialID,
	retiredAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	return f.update(id, func(c *model.MerchantTerminalCredential) {
		c.Status = model.RetiredMerchantTerminalCredentialStatus
		c.RetiredAt = &retiredAt
		c.UpdatedAt = updatedAt
	})
}

func (f *fakeTerminals) MarkRevoked(
	_ context.Context,
	id model.MerchantTerminalCredentialID,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	return f.update(id, func(c *model.MerchantTerminalCredential) {
		c.Status = model.RevokedMerchantTerminalCredentialStatus
		c.UpdatedAt = updatedAt
	})
}

func (f *fakeTerminals) GetIssuanceDeliveryByIdempotencyKey(
	_ context.Context,
	merchantTerminalID model.MerchantTerminalID,
	idempotencyKey string,
) (model.MerchantTerminalIssuanceDelivery, error) {
	for _, delivery := range f.deliveries {
		if delivery.MerchantTerminalID == merchantTerminalID && delivery.IdempotencyKey == idempotencyKey {
			return delivery, nil
		}
	}

	return model.MerchantTerminalIssuanceDelivery{}, notFound()
}

func (f *fakeTerminals) CreateIssuanceDelivery(
	_ context.Context,
	merchantTerminalID model.MerchantTerminalID,
	credentialID model.MerchantTerminalCredentialID,
	idempotencyKey string,
	requestFingerprint string,
	apiKeyCiphertext string,
	apiKeyEncryptionKeyID string,
	expiresAt time.Time,
	createdAt time.Time,
) (model.MerchantTerminalIssuanceDelivery, error) {
	f.writes++

	delivery := model.MerchantTerminalIssuanceDelivery{
		ID:                    model.MerchantTerminalIssuanceDeliveryID(uuid.New()),
		MerchantTerminalID:    merchantTerminalID,
		CredentialID:          credentialID,
		IdempotencyKey:        idempotencyKey,
		RequestFingerprint:    requestFingerprint,
		APIKeyCiphertext:      apiKeyCiphertext,
		APIKeyEncryptionKeyID: apiKeyEncryptionKeyID,
		ExpiresAt:             expiresAt,
		CreatedAt:             createdAt,
	}
	f.deliveries = append(f.deliveries, delivery)

	return delivery, nil
}

func (f *fakeTerminals) ConsumeIssuanceDelivery(
	_ context.Context,
	id model.MerchantTerminalIssuanceDeliveryID,
	consumedAt time.Time,
) (model.MerchantTerminalIssuanceDelivery, error) {
	for index := range f.deliveries {
		if f.deliveries[index].ID == id {
			if f.deliveries[index].ConsumedAt == nil {
				f.deliveries[index].ConsumedAt = &consumedAt
			}

			return f.deliveries[index], nil
		}
	}

	return model.MerchantTerminalIssuanceDelivery{}, notFound()
}

func (f *fakeTerminals) ListPermissions(
	_ context.Context,
	merchantTerminalID model.MerchantTerminalID,
) ([]string, error) {
	if f.permissions == nil {
		return []string{}, nil
	}

	out := append([]string(nil), f.permissions[merchantTerminalID]...)

	return out, nil
}

func (f *fakeTerminals) ReplacePermissions(
	_ context.Context,
	merchantTerminalID model.MerchantTerminalID,
	permissions []string,
	_ time.Time,
) error {
	f.writes++

	if f.permissions == nil {
		f.permissions = make(map[model.MerchantTerminalID][]string)
	}

	seen := make(map[string]struct{}, len(permissions))
	out := make([]string, 0, len(permissions))

	for _, permission := range permissions {
		if permission == "" {
			return fmt.Errorf("permission must not be empty")
		}

		if _, ok := seen[permission]; ok {
			continue
		}

		seen[permission] = struct{}{}
		out = append(out, permission)
	}

	f.permissions[merchantTerminalID] = out

	return nil
}

func (f *fakeTerminals) ScrubExpiredIssuanceDeliveries(
	_ context.Context,
	now time.Time,
	limit int,
) (int64, error) {
	var scrubbed int64

	for index := range f.deliveries {
		if scrubbed >= int64(limit) {
			break
		}

		delivery := &f.deliveries[index]
		if delivery.PurgedAt != nil || delivery.ExpiresAt.After(now) {
			continue
		}

		purgedAt := now
		delivery.APIKeyCiphertext = ""
		delivery.APIKeyEncryptionKeyID = ""
		delivery.PurgedAt = &purgedAt
		scrubbed++
		f.writes++
	}

	return scrubbed, nil
}

// harness wires a UseCase to generated Core mocks. Conn/Tx queryers are backed
// by in-memory state so multi-step terminal lifecycle tests stay readable.
type harness struct {
	t             *testing.T
	ctx           context.Context
	now           time.Time
	pool          *mock.Pool
	terminalsRepo *mock.MerchantTerminal
	profilesRepo  *mock.MerchantProfile
	terminals     *fakeTerminals
	profiles      *fakeMerchantProfiles
	uc            *UseCase
	merchantID    model.MerchantID
}

const (
	testDefaultGrace = 24 * time.Hour
	testMaxGrace     = 72 * time.Hour
	testDeliveryTTL  = 15 * time.Minute
)

func newHarness(t *testing.T) *harness {
	t.Helper()

	hasher, err := fingerprinthmac.New([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)

	box, err := adaptersecretbox.New(adaptersecretbox.Config{
		ActiveKeyID: "k1",
		Keys:        map[string][]byte{"k1": []byte("0123456789abcdef0123456789abcdef")},
	})
	require.NoError(t, err)

	now := time.Date(
		2026,
		time.August,
		9,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	merchantID := model.MerchantID(uuid.New())

	h := &harness{
		t:             t,
		ctx:           context.Background(),
		now:           now,
		pool:          mock.NewPool(t),
		terminalsRepo: mock.NewMerchantTerminal(t),
		profilesRepo:  mock.NewMerchantProfile(t),
		terminals:     &fakeTerminals{},
		merchantID:    merchantID,
		profiles: &fakeMerchantProfiles{
			profiles: map[model.MerchantID]model.MerchantProfile{
				merchantID: {MerchantID: merchantID, Status: model.ActiveMerchantStatus},
			},
		},
	}

	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	h.pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		}).Maybe()
	conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, tx)
		}).Maybe()

	h.profilesRepo.EXPECT().Tx(testifymock.Anything).Return(h.profiles.Tx(nil)).Maybe()
	h.profilesRepo.EXPECT().Conn(testifymock.Anything).
		RunAndReturn(func(repo.Conn) repo.MerchantProfileConnQueryer {
			panic("merchant profile Conn is not used by this use case")
		}).Maybe()

	h.terminalsRepo.EXPECT().Tx(testifymock.Anything).Return(h.terminals).Maybe()
	h.terminalsRepo.EXPECT().Conn(testifymock.Anything).Return(h.terminals).Maybe()

	h.uc, err = New(
		h.pool,
		h.profilesRepo,
		h.terminalsRepo,
		hasher,
		box,
		adapterclock.NewFixed(now, time.UTC),
		Config{
			DefaultGrace:        testDefaultGrace,
			MaxGrace:            testMaxGrace,
			IssuanceDeliveryTTL: testDeliveryTTL,
		},
	)
	require.NoError(t, err)

	return h
}

// advance moves the use case clock forward.
func (h *harness) advance(d time.Duration) {
	h.t.Helper()

	h.now = h.now.Add(d)
	h.uc.clock = adapterclock.NewFixed(h.now, time.UTC)
}

// provision provisions the harness merchant and returns the output.
func (h *harness) provision() ProvisionOutput {
	h.t.Helper()

	out, err := h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID, IdempotencyKey: "provision-1"})
	require.NoError(h.t, err)

	return out
}

func requireCode(t *testing.T, err error, code cerr.Code) {
	t.Helper()

	require.Error(t, err)
	require.True(
		t,
		cerr.IsCode(err, code),
		"expected %s, got %v",
		code,
		err,
	)
}
