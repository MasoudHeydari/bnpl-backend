package trustedmobileuc_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	adapterphonenumber "github.com/parstis/bnpl-backend/pkg/adapter/canonicalizer/phonenumber"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/trustedmobileuc"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Import has no Customer Reference Service dependency by design.
type inMemoryRegistry struct { pairs map[string]string }

func (r *inMemoryRegistry) Conn(repo.Conn) repo.TrustedMobileQueryer { return r }
func (r *inMemoryRegistry) Tx(repo.Tx) repo.TrustedMobileTxQueryer { return r }
func (r *inMemoryRegistry) GetByMobile(_ context.Context, mobile string) (model.TrustedMobileNumber, error) {
	return model.TrustedMobileNumber{MobileE164: mobile, NationalID: r.pairs[mobile]}, nil
}
func (r *inMemoryRegistry) InsertIfAbsent(_ context.Context, mobile, nationalID, _ string, _ time.Time) (bool, error) {
	if old, exists := r.pairs[mobile]; exists {
		if old != nationalID { return false, repo.ErrTrustedMobileNationalIDConflict }
		return false, nil
	}
	r.pairs[mobile] = nationalID
	return true, nil
}

func newImportTestHarness(t *testing.T) (*trustedmobileuc.UseCase, *inMemoryRegistry) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	imports := mock.NewPolicyAssignmentImport(t)
	items := mock.NewPolicyAssignmentImportTxQueryer(t)
	logger := mock.NewLogger(t)
	registry := &inMemoryRegistry{pairs: map[string]string{}}
	importID := model.PolicyAssignmentImportID(uuid.New())
	batch := model.PolicyAssignmentImport{ID: importID, ImportType: "trusted_customers", Status: model.ProcessingPolicyAssignmentImportStatus}

	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
		return fn(ctx, conn)
	}).Maybe()
	conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(func(ctx context.Context, fn repo.TxHandler) error {
		return fn(ctx, tx)
	}).Maybe()
	imports.EXPECT().Tx(tx).Return(items).Maybe()
	items.EXPECT().Create(ctx, "trusted_customers", "trusted.csv", testifymock.Anything, "ops", now).Return(batch, nil).Maybe()
	items.EXPECT().CreateItem(ctx, importID, testifymock.Anything, testifymock.Anything, testifymock.Anything, now).RunAndReturn(
		func(_ context.Context, _ model.PolicyAssignmentImportID, rowNumber int, nationalID, mobileHash *string, _ time.Time) (model.PolicyAssignmentImportItem, error) {
			return model.PolicyAssignmentImportItem{ID: model.PolicyAssignmentImportItemID(uuid.New()), RowNumber: rowNumber}, nil
		}).Maybe()
	items.EXPECT().MarkItemSucceeded(ctx, testifymock.Anything, now).Return(model.PolicyAssignmentImportItem{}, nil).Maybe()
	items.EXPECT().MarkItemFailed(ctx, testifymock.Anything, testifymock.Anything, testifymock.Anything, now).Return(model.PolicyAssignmentImportItem{}, nil).Maybe()
	items.EXPECT().Complete(ctx, importID, testifymock.Anything, testifymock.Anything, testifymock.Anything, now).Return(batch, nil).Maybe()
	logger.EXPECT().With(testifymock.Anything).Return(logger).Maybe()
	logger.EXPECT().Warn(testifymock.Anything).Maybe()
	jurisdictions, err := domaincapability.NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction})
	require.NoError(t, err)
	phones, err := adapterphonenumber.NewRegistry(jurisdictions)
	require.NoError(t, err)
	uc, err := trustedmobileuc.New(pool, registry, imports, phones, adapterclock.NewFixed(now, time.UTC), logger)
	require.NoError(t, err)
	return uc, registry
}

func testImportInput(rows ...trustedmobileuc.ImportCustomer) trustedmobileuc.ImportInput {
	return trustedmobileuc.ImportInput{SourceFile: "trusted.csv", CreatedBy: "ops", Customers: rows}
}

func TestImportAdditiveAndIdempotent(t *testing.T) {
	uc, registry := newImportTestHarness(t)
	input := testImportInput(trustedmobileuc.ImportCustomer{RowNumber: 2, MobileE164: "09121234567", NationalID: "0012345678"})
	first, err := uc.Import(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, first.Added)
	require.Equal(t, "0012345678", registry.pairs["+989121234567"])
	second, err := uc.Import(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, second.AlreadyExists)
	require.Equal(t, 0, second.Added)
	require.Len(t, registry.pairs, 1)
}

func TestImportRejectsNationalIDConflict(t *testing.T) {
	uc, registry := newImportTestHarness(t)
	registry.pairs["+989121234567"] = "0012345678"
	out, err := uc.Import(context.Background(), testImportInput(
		trustedmobileuc.ImportCustomer{RowNumber: 2, MobileE164: "09121234567", NationalID: "9876543210"},
	))
	require.NoError(t, err)
	require.Equal(t, 1, out.Conflicts)
	require.Equal(t, 1, out.FailureCount)
	require.Equal(t, "0012345678", registry.pairs["+989121234567"])
}

func TestImportRejectsInvalidAndRepeatedRows(t *testing.T) {
	uc, _ := newImportTestHarness(t)
	out, err := uc.Import(context.Background(), testImportInput(
		trustedmobileuc.ImportCustomer{RowNumber: 2, MobileE164: "09121234567", NationalID: "0012345678"},
		trustedmobileuc.ImportCustomer{RowNumber: 3, MobileE164: "+989121234567", NationalID: "0012345678"},
		trustedmobileuc.ImportCustomer{RowNumber: 4, MobileE164: "09351234567", NationalID: "123"},
	))
	require.NoError(t, err)
	require.Equal(t, 1, out.Added)
	require.Equal(t, 1, out.Duplicates)
	require.Equal(t, 1, out.InvalidNationalID)
	require.Equal(t, 2, out.FailureCount)
}

func TestImportRejectsConflictingDuplicatesWithinCSV(t *testing.T) {
	uc, _ := newImportTestHarness(t)
	out, err := uc.Import(context.Background(), testImportInput(
		trustedmobileuc.ImportCustomer{RowNumber: 2, MobileE164: "09121234567", NationalID: "0012345678"},
		trustedmobileuc.ImportCustomer{RowNumber: 3, MobileE164: "+989121234567", NationalID: "9876543210"},
	))
	require.NoError(t, err)
	require.Equal(t, 1, out.Conflicts)
	require.Equal(t, 1, out.FailureCount)
}
