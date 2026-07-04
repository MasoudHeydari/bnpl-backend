package otp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
)

func requireMessageKey(t *testing.T, err error, key cerr.MessageKey) {
	t.Helper()

	var appErr *cerr.Error
	require.Error(t, err)
	require.True(t, errors.As(err, &appErr), "error=%v", err)
	require.Equal(t, key, appErr.MessageKey)
}

func TestVerifyAndConsume_SingleUse(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.request()
	require.NoError(t, err)

	code := mustCodeFromSubmit(t, h.notification)
	first, err := h.verify(code)
	require.NoError(t, err)
	require.NotNil(t, first.Challenge.VerifiedAt)
	require.Nil(t, first.Challenge.Verifier)
	require.NotNil(t, first.Challenge.NextIssueAt)

	_, err = h.verify(code)
	requireMessageKey(t, err, "error.otp_already_consumed")
}

func TestVerifyAndConsume_WrongCodePersistsAttempt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.request()
	require.NoError(t, err)

	_, err = h.verify("000000")
	requireMessageKey(t, err, "error.otp_invalid")

	row, getErr := h.challenges.GetByPurposeSubject(h.ctx, testPurpose, testSubjectKey)
	require.NoError(t, getErr)
	require.Equal(t, 1, row.AttemptCount)
	require.NotNil(t, row.Verifier)
}

func TestVerifyAndConsume_Lockout(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.request()
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		_, err = h.verify("000000")
		require.Error(t, err)
	}

	requireMessageKey(t, err, "error.otp_locked")

	code := mustCodeFromSubmit(t, h.notification)
	_, err = h.verify(code)
	requireMessageKey(t, err, "error.otp_locked")
}

func TestVerifyAndConsume_Expired(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.request()
	require.NoError(t, err)

	code := mustCodeFromSubmit(t, h.notification)
	h.advance(2 * time.Minute)

	_, err = h.verify(code)
	requireMessageKey(t, err, "error.otp_expired")

	row, getErr := h.challenges.GetByPurposeSubject(h.ctx, testPurpose, testSubjectKey)
	require.NoError(t, getErr)
	require.Equal(t, 0, row.AttemptCount)
}

func TestVerifyAndConsume_ContextVersionIsolation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.request()
	require.NoError(t, err)

	code := mustCodeFromSubmit(t, h.notification)
	_, err = h.uc.VerifyAndConsume(h.ctx, otp.VerifyInput{
		Purpose:        testPurpose,
		SubjectKey:     testSubjectKey,
		ContextVersion: "2",
		Binding:        testBinding,
		Code:           code,
	})
	requireMessageKey(t, err, "error.otp_context_invalidated")

	row, getErr := h.challenges.GetByPurposeSubject(h.ctx, testPurpose, testSubjectKey)
	require.NoError(t, getErr)
	require.Equal(t, 0, row.AttemptCount)
	require.NotNil(t, row.Verifier)
}

func TestVerifyAndConsume_SubjectIsolation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.request()
	require.NoError(t, err)

	code := mustCodeFromSubmit(t, h.notification)
	_, err = h.uc.VerifyAndConsume(h.ctx, otp.VerifyInput{
		Purpose:        testPurpose,
		SubjectKey:     "cs_other_subject_key_0000000000000000000000000000000000000000000000000000",
		ContextVersion: testContextVersion,
		Binding:        testBinding,
		Code:           code,
	})
	requireMessageKey(t, err, "error.otp_not_requested")
}

func TestVerifyAndConsume_MalformedRejected(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.uc.VerifyAndConsume(h.ctx, otp.VerifyInput{
		Purpose:        testPurpose,
		SubjectKey:     testSubjectKey,
		ContextVersion: testContextVersion,
		Binding:        testBinding,
		Code:           "12ab56",
	})
	requireMessageKey(t, err, "error.otp_invalid")
}

func TestVerifyAndConsume_EmptyCodeRequired(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.uc.VerifyAndConsume(h.ctx, otp.VerifyInput{
		Purpose:        testPurpose,
		SubjectKey:     testSubjectKey,
		ContextVersion: testContextVersion,
		Binding:        testBinding,
		Code:           "",
	})
	requireMessageKey(t, err, "error.otp_required")
}

func TestVerifyAndConsumeInTx_ComposesWithCallerTx(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.request()
	require.NoError(t, err)

	code := mustCodeFromSubmit(t, h.notification)

	var out otp.VerifyOutput
	err = h.pool.Conn(h.ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			result, rejection, verifyErr := h.uc.VerifyAndConsumeInTx(ctx, tx, otp.VerifyInput{
				Purpose:        testPurpose,
				SubjectKey:     testSubjectKey,
				ContextVersion: testContextVersion,
				Binding:        testBinding,
				Code:           code,
			})
			require.NoError(t, verifyErr)
			require.NoError(t, rejection)
			out = result

			return nil
		})
	})
	require.NoError(t, err)
	require.NotNil(t, out.Challenge.VerifiedAt)
}

type blockingConsumeAuthorizer struct {
	err error
	n   int
}

func (a *blockingConsumeAuthorizer) AuthorizeConsume(context.Context, repo.Tx) error {
	a.n++

	return a.err
}

func TestVerifyAndConsumeInTx_ConsumeAuthorizerBlocksLeavesChallengeUsable(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.request()
	require.NoError(t, err)

	code := mustCodeFromSubmit(t, h.notification)
	blocker := &blockingConsumeAuthorizer{
		err: cerr.NewPublic(
			cerr.CodeConflict,
			"error.otp_issuance_not_allowed",
			errors.New("manual review"),
			cerr.Details{},
		),
	}

	err = h.pool.Conn(h.ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			_, rejection, verifyErr := h.uc.VerifyAndConsumeInTx(ctx, tx, otp.VerifyInput{
				Purpose:           testPurpose,
				SubjectKey:        testSubjectKey,
				ContextVersion:    testContextVersion,
				Binding:           testBinding,
				Code:              code,
				ConsumeAuthorizer: blocker,
			})
			require.NoError(t, rejection)
			require.Error(t, verifyErr)
			requireMessageKey(t, verifyErr, "error.otp_issuance_not_allowed")

			return verifyErr
		})
	})
	require.Error(t, err)
	require.Equal(t, 1, blocker.n)

	row, getErr := h.challenges.GetByPurposeSubject(h.ctx, testPurpose, testSubjectKey)
	require.NoError(t, getErr)
	require.Nil(t, row.VerifiedAt)
	require.NotNil(t, row.Verifier)
	require.Equal(t, 0, row.AttemptCount)
}

func TestVerifyAndConsumeInTx_WrongCodeSkipsConsumeAuthorizer(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.request()
	require.NoError(t, err)

	blocker := &blockingConsumeAuthorizer{
		err: errors.New("must not run"),
	}

	var rejection error
	err = h.pool.Conn(h.ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			_, reject, verifyErr := h.uc.VerifyAndConsumeInTx(ctx, tx, otp.VerifyInput{
				Purpose:           testPurpose,
				SubjectKey:        testSubjectKey,
				ContextVersion:    testContextVersion,
				Binding:           testBinding,
				Code:              "000000",
				ConsumeAuthorizer: blocker,
			})
			rejection = reject

			return verifyErr
		})
	})
	require.NoError(t, err)
	requireMessageKey(t, rejection, "error.otp_invalid")
	require.Equal(t, 0, blocker.n)

	row, getErr := h.challenges.GetByPurposeSubject(h.ctx, testPurpose, testSubjectKey)
	require.NoError(t, getErr)
	require.Nil(t, row.VerifiedAt)
	require.NotNil(t, row.Verifier)
	require.Equal(t, 1, row.AttemptCount)
}
