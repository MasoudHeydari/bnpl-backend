package policyassignmentuc_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	adapterphonenumber "github.com/parstis/bnpl-backend/pkg/adapter/canonicalizer/phonenumber"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	fingerprinthmac "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/hmac"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/policyassignmentuc"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestImportTrustedOutput_FormatSummary(t *testing.T) {
	t.Parallel()

	out := policyassignmentuc.ImportTrustedOutput{
		Added:                 2,
		AlreadyExists:         1,
		NotFound:              1,
		InvalidCustomerStatus: 1,
		InvalidMobile:         1,
		Duplicates:            1,
		CustomerServiceErrors: 1,
		DatabaseErrors:        1,
	}
	summary := out.FormatSummary()
	require.Contains(t, summary, "Trusted Customers Import Summary")
	require.Contains(t, summary, "Added:")
	require.Contains(t, summary, "Processed:")
	require.True(t, strings.Contains(summary, "9") || strings.Contains(summary, "   9"))
}

func TestImportTrusted_ActiveCustomerAdded(t *testing.T) {
	t.Parallel()

	h := newImportHarness(t)
	ctx := context.Background()
	externalID := mustExternalCustomerID(t, "550e8400-e29b-41d4-a716-446655440000")
	mobile := "+989121234567"

	h.expectCreateBatch(ctx, 1)
	h.customers.EXPECT().ResolveByMobile(ctx, extcustomer.ResolveByMobileInput{MobileE164: mobile}).
		Return(extcustomer.ResolveByMobileOutput{
			ExternalCustomerID: externalID,
			Status:             extcustomer.ActiveStatus,
		}, nil).Once()
	h.expectAssignAdded(ctx, externalID, 2)
	h.expectComplete(ctx, 1, 0)

	out, err := h.uc.ImportTrusted(ctx, policyassignmentuc.ImportTrustedInput{
		SourceFile: "trusted.csv",
		CreatedBy:  "ops@example",
		Customers: []policyassignmentuc.ImportCustomer{
			{RowNumber: 2, MobileE164: mobile},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.Added)
	require.Equal(t, 0, out.AlreadyExists)
	require.Equal(t, 1, out.SuccessCount)
	require.Equal(t, 0, out.FailureCount)
}

func TestImportTrusted_AlreadyExists(t *testing.T) {
	t.Parallel()

	h := newImportHarness(t)
	ctx := context.Background()
	externalID := mustExternalCustomerID(t, "550e8400-e29b-41d4-a716-446655440000")
	mobile := "+989121234567"

	h.expectCreateBatch(ctx, 1)
	h.customers.EXPECT().ResolveByMobile(ctx, extcustomer.ResolveByMobileInput{MobileE164: mobile}).
		Return(extcustomer.ResolveByMobileOutput{
			ExternalCustomerID: externalID,
			Status:             extcustomer.ActiveStatus,
		}, nil).Once()
	h.expectAssignAlreadyExists(ctx, externalID, 2)
	h.expectComplete(ctx, 1, 0)

	out, err := h.uc.ImportTrusted(ctx, policyassignmentuc.ImportTrustedInput{
		SourceFile: "trusted.csv",
		CreatedBy:  "ops@example",
		Customers: []policyassignmentuc.ImportCustomer{
			{RowNumber: 2, MobileE164: mobile},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 0, out.Added)
	require.Equal(t, 1, out.AlreadyExists)
}

func TestImportTrusted_SuspendedStatus(t *testing.T) {
	t.Parallel()

	h := newImportHarness(t)
	ctx := context.Background()
	externalID := mustExternalCustomerID(t, "550e8400-e29b-41d4-a716-446655440000")
	mobile := "+989121234567"

	h.expectCreateBatch(ctx, 1)
	h.customers.EXPECT().ResolveByMobile(ctx, testifymock.Anything).
		Return(extcustomer.ResolveByMobileOutput{
			ExternalCustomerID: externalID,
			Status:             extcustomer.SuspendedStatus,
		}, nil).Once()
	h.expectFailedItem(ctx, 2)
	h.expectComplete(ctx, 0, 1)

	out, err := h.uc.ImportTrusted(ctx, policyassignmentuc.ImportTrustedInput{
		SourceFile: "trusted.csv",
		CreatedBy:  "ops@example",
		Customers: []policyassignmentuc.ImportCustomer{
			{RowNumber: 2, MobileE164: mobile},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.InvalidCustomerStatus)
	require.Equal(t, 1, out.FailureCount)
}

func TestImportTrusted_NotFound(t *testing.T) {
	t.Parallel()

	h := newImportHarness(t)
	ctx := context.Background()
	mobile := "+989121234567"

	h.expectCreateBatch(ctx, 1)
	h.customers.EXPECT().ResolveByMobile(ctx, testifymock.Anything).
		Return(extcustomer.ResolveByMobileOutput{}, cerr.ResourceNotFound(errors.New("missing"))).Once()
	h.expectFailedItem(ctx, 2)
	h.expectComplete(ctx, 0, 1)

	out, err := h.uc.ImportTrusted(ctx, policyassignmentuc.ImportTrustedInput{
		SourceFile: "trusted.csv",
		CreatedBy:  "ops@example",
		Customers: []policyassignmentuc.ImportCustomer{
			{RowNumber: 2, MobileE164: mobile},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.NotFound)
}

func TestImportTrusted_ResolveTimeout(t *testing.T) {
	t.Parallel()

	h := newImportHarness(t)
	ctx := context.Background()
	mobile := "+989121234567"

	h.expectCreateBatch(ctx, 1)
	h.customers.EXPECT().ResolveByMobile(ctx, testifymock.Anything).
		Return(extcustomer.ResolveByMobileOutput{}, cerr.ServiceUnavailable(errors.New("timeout"))).Once()
	h.expectFailedItem(ctx, 2)
	h.expectComplete(ctx, 0, 1)

	out, err := h.uc.ImportTrusted(ctx, policyassignmentuc.ImportTrustedInput{
		SourceFile: "trusted.csv",
		CreatedBy:  "ops@example",
		Customers: []policyassignmentuc.ImportCustomer{
			{RowNumber: 2, MobileE164: mobile},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.CustomerServiceErrors)
}

func TestImportTrusted_InvalidMobile(t *testing.T) {
	t.Parallel()

	h := newImportHarness(t)
	ctx := context.Background()

	h.expectCreateBatch(ctx, 1)
	h.expectFailedItem(ctx, 2)
	h.expectComplete(ctx, 0, 1)

	out, err := h.uc.ImportTrusted(ctx, policyassignmentuc.ImportTrustedInput{
		SourceFile: "trusted.csv",
		CreatedBy:  "ops@example",
		Customers: []policyassignmentuc.ImportCustomer{
			{RowNumber: 2, MobileE164: "not-a-phone"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.InvalidMobile)
}

func TestImportTrusted_DuplicateMobileInCSV(t *testing.T) {
	t.Parallel()

	h := newImportHarness(t)
	ctx := context.Background()
	externalID := mustExternalCustomerID(t, "550e8400-e29b-41d4-a716-446655440000")
	mobile := "+989121234567"

	h.expectCreateBatch(ctx, 2)
	h.customers.EXPECT().ResolveByMobile(ctx, testifymock.Anything).
		Return(extcustomer.ResolveByMobileOutput{
			ExternalCustomerID: externalID,
			Status:             extcustomer.ActiveStatus,
		}, nil).Once()
	h.expectAssignAdded(ctx, externalID, 2)
	h.expectFailedItem(ctx, 3)
	h.expectComplete(ctx, 1, 1)

	out, err := h.uc.ImportTrusted(ctx, policyassignmentuc.ImportTrustedInput{
		SourceFile: "trusted.csv",
		CreatedBy:  "ops@example",
		Customers: []policyassignmentuc.ImportCustomer{
			{RowNumber: 2, MobileE164: mobile},
			{RowNumber: 3, MobileE164: "09121234567"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.Added)
	require.Equal(t, 1, out.Duplicates)
}

func TestImportTrusted_DatabaseFailureDoesNotStopNextRow(t *testing.T) {
	t.Parallel()

	h := newImportHarness(t)
	ctx := context.Background()
	failID := mustExternalCustomerID(t, "550e8400-e29b-41d4-a716-446655440000")
	okID := mustExternalCustomerID(t, "92d2c7b2-6f8b-4f25-8bb2-4deab49af151")

	h.expectCreateBatch(ctx, 2)
	h.customers.EXPECT().ResolveByMobile(ctx, extcustomer.ResolveByMobileInput{MobileE164: "+989121234567"}).
		Return(extcustomer.ResolveByMobileOutput{ExternalCustomerID: failID, Status: extcustomer.ActiveStatus}, nil).Once()
	h.customers.EXPECT().ResolveByMobile(ctx, extcustomer.ResolveByMobileInput{MobileE164: "+989351234567"}).
		Return(extcustomer.ResolveByMobileOutput{ExternalCustomerID: okID, Status: extcustomer.ActiveStatus}, nil).Once()

	// First row: CreateItem succeeds, LockActive fails → TX rolls back, then durable failure item.
	h.passthroughConnTx(ctx)
	h.imports.EXPECT().Tx(h.tx).Return(h.importsTx).Once()
	failItemID := model.PolicyAssignmentImportItemID(uuid.New())
	failSubject := failID.String()
	h.importsTx.EXPECT().CreateItem(ctx, h.importID, 2, testifymock.Anything, testifymock.Anything, h.now).
		Return(model.PolicyAssignmentImportItem{
			ID:        failItemID,
			ImportID:  h.importID,
			RowNumber: 2,
			SubjectID: &failSubject,
			Status:    model.PendingPolicyAssignmentImportItemStatus,
		}, nil).Once()
	h.assignments.EXPECT().Tx(h.tx).Return(h.assignmentTx).Once()
	h.assignmentTx.EXPECT().LockActive(
		ctx,
		model.CustomerPolicySubjectType,
		failSubject,
		testifymock.Anything,
		h.now,
	).Return(model.PolicyAssignment{}, errors.New("db lock failed")).Once()
	h.expectFailedItem(ctx, 2)

	h.expectAssignAdded(ctx, okID, 3)
	h.expectComplete(ctx, 1, 1)

	out, err := h.uc.ImportTrusted(ctx, policyassignmentuc.ImportTrustedInput{
		SourceFile: "trusted.csv",
		CreatedBy:  "ops@example",
		Customers: []policyassignmentuc.ImportCustomer{
			{RowNumber: 2, MobileE164: "+989121234567"},
			{RowNumber: 3, MobileE164: "+989351234567"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.DatabaseErrors)
	require.Equal(t, 1, out.Added)
}

func TestImportTrusted_MobileHashDeterministic(t *testing.T) {
	t.Parallel()

	hasher, err := fingerprinthmac.New([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)

	first, err := hasher.Hash("+989121234567")
	require.NoError(t, err)
	second, err := hasher.Hash("+989121234567")
	require.NoError(t, err)
	require.Equal(t, first.String(), second.String())
	require.True(t, strings.HasPrefix(first.String(), "hmac-sha256:"))
}

type importHarness struct {
	t            *testing.T
	uc           *policyassignmentuc.UseCase
	pool         *mock.Pool
	conn         *mock.Conn
	tx           *mock.Tx
	assignments  *mock.PolicyAssignment
	assignmentTx *mock.PolicyAssignmentTxQueryer
	imports      *mock.PolicyAssignmentImport
	importsTx    *mock.PolicyAssignmentImportTxQueryer
	customers    *mock.CustomerResolver
	now          time.Time
	importID     model.PolicyAssignmentImportID
}

func newImportHarness(t *testing.T) *importHarness {
	t.Helper()

	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	jurisdictions, err := domaincapability.NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction})
	require.NoError(t, err)
	phones, err := adapterphonenumber.NewRegistry(jurisdictions)
	require.NoError(t, err)
	hasher, err := fingerprinthmac.New([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)

	h := &importHarness{
		t:            t,
		pool:         mock.NewPool(t),
		conn:         mock.NewConn(t),
		tx:           mock.NewTx(t),
		assignments:  mock.NewPolicyAssignment(t),
		assignmentTx: mock.NewPolicyAssignmentTxQueryer(t),
		imports:      mock.NewPolicyAssignmentImport(t),
		importsTx:    mock.NewPolicyAssignmentImportTxQueryer(t),
		customers:    mock.NewCustomerResolver(t),
		now:          now,
		importID:     model.PolicyAssignmentImportID(uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")),
	}

	logger := mock.NewLogger(t)
	logger.EXPECT().Info(testifymock.Anything, testifymock.Anything).Maybe()
	logger.EXPECT().Warn(testifymock.Anything, testifymock.Anything).Maybe()
	logger.EXPECT().Error(testifymock.Anything, testifymock.Anything).Maybe()
	logger.EXPECT().With(testifymock.Anything).Return(logger).Maybe()

	uc, err := policyassignmentuc.New(
		h.pool,
		h.assignments,
		h.imports,
		h.customers,
		phones,
		hasher,
		adapterclock.NewFixed(now, time.UTC),
		logger,
	)
	require.NoError(t, err)
	h.uc = uc
	return h
}

func (h *importHarness) passthroughConnTx(ctx context.Context) {
	h.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, h.conn)
		},
	).Once()
	h.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, h.tx)
		},
	).Once()
}

func (h *importHarness) expectCreateBatch(ctx context.Context, total int) {
	h.passthroughConnTx(ctx)
	h.imports.EXPECT().Tx(h.tx).Return(h.importsTx).Once()
	h.importsTx.EXPECT().Create(
		ctx,
		"trusted_customers",
		"trusted.csv",
		total,
		"ops@example",
		h.now,
	).Return(model.PolicyAssignmentImport{
		ID:         h.importID,
		ImportType: "trusted_customers",
		SourceFile: "trusted.csv",
		Status:     model.ProcessingPolicyAssignmentImportStatus,
		TotalCount: total,
		CreatedBy:  "ops@example",
		CreatedAt:  h.now,
	}, nil).Once()
}

func (h *importHarness) expectComplete(ctx context.Context, success, failure int) {
	h.passthroughConnTx(ctx)
	h.imports.EXPECT().Tx(h.tx).Return(h.importsTx).Once()
	status := model.CompletedPolicyAssignmentImportStatus
	if failure > 0 {
		status = model.CompletedWithFailuresPolicyAssignmentImportStatus
	}

	completedAt := h.now
	h.importsTx.EXPECT().Complete(ctx, h.importID, status, success, failure, completedAt).
		Return(model.PolicyAssignmentImport{
			ID:           h.importID,
			Status:       status,
			TotalCount:   success + failure,
			SuccessCount: success,
			FailureCount: failure,
			CompletedAt:  &completedAt,
		}, nil).Once()
}

func (h *importHarness) expectFailedItem(ctx context.Context, rowNumber int) {
	h.passthroughConnTx(ctx)
	h.imports.EXPECT().Tx(h.tx).Return(h.importsTx).Once()
	itemID := model.PolicyAssignmentImportItemID(uuid.New())
	h.importsTx.EXPECT().CreateItem(
		ctx,
		h.importID,
		rowNumber,
		testifymock.Anything,
		testifymock.Anything,
		h.now,
	).Return(model.PolicyAssignmentImportItem{
		ID:        itemID,
		ImportID:  h.importID,
		RowNumber: rowNumber,
		Status:    model.PendingPolicyAssignmentImportItemStatus,
	}, nil).Once()
	h.importsTx.EXPECT().MarkItemFailed(
		ctx,
		itemID,
		testifymock.Anything,
		testifymock.Anything,
		h.now,
	).Return(model.PolicyAssignmentImportItem{
		ID:     itemID,
		Status: model.FailedPolicyAssignmentImportItemStatus,
	}, nil).Once()
}

func (h *importHarness) expectAssignAdded(ctx context.Context, externalID model.ExternalCustomerID, rowNumber int) {
	h.passthroughConnTx(ctx)
	h.imports.EXPECT().Tx(h.tx).Return(h.importsTx).Once()
	itemID := model.PolicyAssignmentImportItemID(uuid.New())
	subject := externalID.String()
	h.importsTx.EXPECT().CreateItem(
		ctx,
		h.importID,
		rowNumber,
		testifymock.Anything,
		testifymock.Anything,
		h.now,
	).Return(model.PolicyAssignmentImportItem{
		ID:        itemID,
		ImportID:  h.importID,
		RowNumber: rowNumber,
		SubjectID: &subject,
		Status:    model.PendingPolicyAssignmentImportItemStatus,
	}, nil).Once()

	h.assignments.EXPECT().Tx(h.tx).Return(h.assignmentTx).Once()
	for range policy.TrustedCustomerBundle() {
		h.assignmentTx.EXPECT().LockActive(
			ctx,
			model.CustomerPolicySubjectType,
			subject,
			testifymock.Anything,
			h.now,
		).Return(model.PolicyAssignment{}, cerr.ResourceNotFound(errors.New("none"))).Once()
		h.assignmentTx.EXPECT().Create(
			ctx,
			model.CustomerPolicySubjectType,
			subject,
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			"trusted_customer",
			"csv_import",
			h.now,
			"ops@example",
			h.now,
			h.now,
		).Return(model.PolicyAssignment{ID: model.PolicyAssignmentID(uuid.New())}, nil).Once()
	}

	h.importsTx.EXPECT().MarkItemSucceeded(ctx, itemID, h.now).
		Return(model.PolicyAssignmentImportItem{ID: itemID, Status: model.SucceededPolicyAssignmentImportItemStatus}, nil).Once()
}

func (h *importHarness) expectAssignAlreadyExists(ctx context.Context, externalID model.ExternalCustomerID, rowNumber int) {
	h.passthroughConnTx(ctx)
	h.imports.EXPECT().Tx(h.tx).Return(h.importsTx).Once()
	itemID := model.PolicyAssignmentImportItemID(uuid.New())
	subject := externalID.String()
	h.importsTx.EXPECT().CreateItem(
		ctx,
		h.importID,
		rowNumber,
		testifymock.Anything,
		testifymock.Anything,
		h.now,
	).Return(model.PolicyAssignmentImportItem{
		ID:        itemID,
		ImportID:  h.importID,
		RowNumber: rowNumber,
		SubjectID: &subject,
		Status:    model.PendingPolicyAssignmentImportItemStatus,
	}, nil).Once()

	h.assignments.EXPECT().Tx(h.tx).Return(h.assignmentTx).Once()
	for _, target := range policy.TrustedCustomerBundle() {
		h.assignmentTx.EXPECT().LockActive(
			ctx,
			model.CustomerPolicySubjectType,
			subject,
			target.Family,
			h.now,
		).Return(model.PolicyAssignment{
			ID:            model.PolicyAssignmentID(uuid.New()),
			PolicyID:      target.Ref.ID,
			PolicyVersion: target.Ref.Version,
		}, nil).Once()
	}

	h.importsTx.EXPECT().MarkItemSucceeded(ctx, itemID, h.now).
		Return(model.PolicyAssignmentImportItem{ID: itemID, Status: model.SucceededPolicyAssignmentImportItemStatus}, nil).Once()
}

func mustExternalCustomerID(t *testing.T, raw string) model.ExternalCustomerID {
	t.Helper()
	id, err := model.ParseExternalCustomerID(raw)
	require.NoError(t, err)
	return id
}
