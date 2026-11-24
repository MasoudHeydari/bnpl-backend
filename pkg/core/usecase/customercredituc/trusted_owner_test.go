package customercredituc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/stretchr/testify/require"
)

type trustedMobileStub struct {
	mobile     string
	nationalID string
}

var _ repo.TrustedMobile = trustedMobileStub{}

func (s trustedMobileStub) Conn(repo.Conn) repo.TrustedMobileQueryer {
	return s
}

func (s trustedMobileStub) Tx(repo.Tx) repo.TrustedMobileTxQueryer {
	return s
}

func (s trustedMobileStub) GetByMobile(_ context.Context, mobile string) (model.TrustedMobileNumber, error) {
	if mobile != s.mobile {
		return model.TrustedMobileNumber{}, cerr.ResourceNotFound(errors.New("trusted phone not found"))
	}

	return model.TrustedMobileNumber{
		MobileE164: s.mobile,
		NationalID: s.nationalID,
	}, nil
}

func (trustedMobileStub) InsertIfAbsent(context.Context, string, string, string, time.Time) (bool, error) {
	return false, errors.New("unexpected trusted CSV write during credit assignment")
}

func TestTrustedMobileNationalIDMatchesAuthoritativeCRS(t *testing.T) {
	t.Parallel()

	uc := &UseCase{trustedMobile: trustedMobileStub{
		mobile:     "+989121234567",
		nationalID: "0012345678",
	}}

	eligible, err := uc.eligibleTrustedOwner(
		context.Background(),
		mock.NewTx(t),
		"+989121234567",
		"0012345678",
	)
	require.NoError(t, err)
	require.True(t, eligible)
}

func TestTrustedMobileNationalIDConflictDoesNotFallBackToOrdinaryCredit(t *testing.T) {
	t.Parallel()

	uc := &UseCase{trustedMobile: trustedMobileStub{
		mobile:     "+989121234567",
		nationalID: "0012345678",
	}}

	eligible, err := uc.eligibleTrustedOwner(
		context.Background(),
		mock.NewTx(t),
		"+989121234567",
		"9876543210",
	)
	require.False(t, eligible)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}

func TestNormalMobileIsNotTrusted(t *testing.T) {
	t.Parallel()

	uc := &UseCase{trustedMobile: trustedMobileStub{
		mobile:     "+989121234567",
		nationalID: "0012345678",
	}}

	eligible, err := uc.eligibleTrustedOwner(
		context.Background(),
		mock.NewTx(t),
		"+989351234567",
		"1234567891",
	)
	require.NoError(t, err)
	require.False(t, eligible)
}
