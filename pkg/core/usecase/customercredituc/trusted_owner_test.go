package customercredituc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	testifyMock "github.com/stretchr/testify/mock"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
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

func (trustedMobileStub) InsertIfAbsent(
	context.Context,
	string,
	string,
	string,
	time.Time,
) (bool, error) {
	return false, errors.New("unexpected trusted CSV write during credit assignment")
}

func TestTrustedMobileNationalIDMatchesAuthoritativeCRS(t *testing.T) {
	t.Parallel()

	uc := &UseCase{logger: mock.NewLogger(t), trustedMobile: trustedMobileStub{
		mobile:     "+989121234567",
		nationalID: "0012345678",
	}}

	eligible, err := uc.eligibleTrustedOwner(
		context.Background(),
		mock.NewTx(t),
		model.CustomerProfile{},
		"+989121234567",
		"0012345678",
	)
	require.NoError(t, err)
	require.True(t, eligible)
}

func TestTrustedMobileNationalIDConflictDoesNotFallBackToOrdinaryCredit(t *testing.T) {
	t.Parallel()

	logger := mock.NewLogger(t)
	uc := &UseCase{logger: logger, trustedMobile: trustedMobileStub{
		mobile:     "+989121234567",
		nationalID: "0012345678",
	}}

	logger.EXPECT().WithContext(context.Background()).Return(logger).Once()
	logger.EXPECT().Error(
		testifyMock.MatchedBy(func(event log.Event) bool {
			return event.Code() == "credit.trusted_mobile_national_id_conflict"
		}),
		testifyMock.MatchedBy(func(fields []log.Field) bool {
			if len(fields) != 3 {
				return false
			}
			return fields[0].Key() == "customer_id" &&
				fields[1].Key() == "external_customer_id" &&
				fields[2].Key() == "reason_code" &&
				fields[2].Value() == "TRUSTED_MOBILE_NATIONAL_ID_CONFLICT"
		}),
	).Once()

	eligible, err := uc.eligibleTrustedOwner(
		context.Background(),
		mock.NewTx(t),
		model.CustomerProfile{},
		"+989121234567",
		"9876543210",
	)
	require.False(t, eligible)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}

func TestNormalMobileIsNotTrusted(t *testing.T) {
	t.Parallel()

	uc := &UseCase{logger: mock.NewLogger(t), trustedMobile: trustedMobileStub{
		mobile:     "+989121234567",
		nationalID: "0012345678",
	}}

	eligible, err := uc.eligibleTrustedOwner(
		context.Background(),
		mock.NewTx(t),
		model.CustomerProfile{},
		"+989351234567",
		"1234567891",
	)
	require.NoError(t, err)
	require.False(t, eligible)
}
