package bootstrap

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
)

// countingUUIDGenerator counts Generate calls while emitting UUID text.
// Used to prove NewUseCases closed over DomainUUIDGenerator, not the ULID Trace provider.
type countingUUIDGenerator struct {
	calls atomic.Int64
}

func (g *countingUUIDGenerator) Generate() string {
	g.calls.Add(1)
	return uuid.NewString()
}

// TestNewUseCases_WiresDomainUUIDGeneratorIntoUUIDDependentUseCases exercises the
// real Bootstrap composition path and fails if ShortLink / RepaymentReminder /
// Collections are wired back to IdentifierGenerator (ULID Trace).
func TestNewUseCases_WiresDomainUUIDGeneratorIntoUUIDDependentUseCases(t *testing.T) {
	t.Parallel()

	t.Run("source_injections_use_DomainUUIDGenerator", func(t *testing.T) {
		t.Parallel()
		assertUseCaseConstructorsWireDomainUUIDGenerator(t)
	})

	t.Run("shortlink_create_uses_DomainUUIDGenerator", func(t *testing.T) {
		t.Parallel()
		assertShortLinkCreateUsesDomainUUIDGenerator(t)
	})
}

func assertUseCaseConstructorsWireDomainUUIDGenerator(t *testing.T) {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	srcPath := filepath.Join(filepath.Dir(thisFile), "usecase.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, srcPath, nil, 0)
	require.NoError(t, err)

	wantPkgs := map[string]bool{
		"collectionsuc":       false,
		"shortlinkuc":         false,
		"repaymentreminderuc": false,
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || sel.Sel.Name != "New" {
			return true
		}

		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		pkgName := pkgIdent.Name
		if _, tracked := wantPkgs[pkgName]; !tracked {
			return true
		}

		require.Truef(
			t,
			callUsesDomainUUIDGenerator(call),
			"%s.New must receive a.Infrastructure.DomainUUIDGenerator (not IdentifierGenerator)",
			pkgName,
		)
		wantPkgs[pkgName] = true
		return true
	})

	for pkgName, seen := range wantPkgs {
		require.Truef(t, seen, "expected to find %s.New in usecase.go", pkgName)
	}
}

func callUsesDomainUUIDGenerator(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		// Expect argument shape: a.Infrastructure.DomainUUIDGenerator
		sel, ok := arg.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || sel.Sel.Name != "DomainUUIDGenerator" {
			continue
		}

		infra, ok := sel.X.(*ast.SelectorExpr)
		if !ok || infra.Sel == nil || infra.Sel.Name != "Infrastructure" {
			continue
		}

		app, ok := infra.X.(*ast.Ident)
		if ok && app.Name == "a" {
			return true
		}
	}

	return false
}

func assertShortLinkCreateUsesDomainUUIDGenerator(t *testing.T) {
	t.Helper()

	cfg := noopTestConfig()
	cfg.Usecases.ShortLink.AllowedDestinationHosts = []string{"frontend.bnpl.test"}
	cfg.Usecases.ShortLink.PublicHost = "short.bnpl.test"
	cfg.Usecases.ShortLink.RepaymentDestinationURL = "https://frontend.bnpl.test/repayment"

	app := testAppWithInfrastructure(t, cfg)

	domainGen := &countingUUIDGenerator{}
	app.Infrastructure.DomainUUIDGenerator = domainGen

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	shortLinkRepo := mock.NewShortLink(t)
	shortLinkTxQ := mock.NewShortLinkTxQueryer(t)

	pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, conn)
		}).Maybe()
	conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.TxHandler) error {
			return fn(ctx, tx)
		}).Maybe()
	shortLinkRepo.EXPECT().Tx(tx).Return(shortLinkTxQ).Maybe()
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
			return model.ShortLink{
				ID:             id,
				Code:           code,
				DestinationURL: destination,
				CreatedAt:      createdAt,
				UpdatedAt:      updatedAt,
			}, nil
		}).Maybe()

	app.Pool = pool
	app.Repos.ShortLink = shortLinkRepo

	usecases, err := app.NewUseCases()
	require.NoError(t, err)
	require.NotNil(t, usecases.ShortLink)
	require.NotNil(t, usecases.RepaymentReminder)
	require.NotNil(t, usecases.Collections)

	before := domainGen.calls.Load()
	out, err := usecases.ShortLink.Create(context.Background(), shortlinkuc.CreateInput{
		DestinationURL: "https://frontend.bnpl.test/repayment",
	})
	require.NoError(t, err)
	require.Greater(t, domainGen.calls.Load(), before,
		"ShortLink must call DomainUUIDGenerator; wiring IdentifierGenerator would leave this counter unchanged")

	parsed, err := uuid.Parse(out.ShortLink.ID.String())
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, parsed)

	// Opaque Trace / IdentifierGenerator must remain ULID-shaped.
	opaque := app.Infrastructure.IdentifierGenerator.Generate()
	_, err = uuid.Parse(opaque)
	require.Error(t, err)
	require.Len(t, opaque, 26)
}
