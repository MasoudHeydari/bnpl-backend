package otp_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	otppolicy "github.com/parstis/bnpl-backend/pkg/core/policy/otp"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
)

func TestProjectAvailability_Priority(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		time.March,
		20,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	expires := now.Add(time.Minute)
	next := now.Add(2 * time.Minute)
	pastNext := now.Add(-time.Second)
	issueID := model.OTPIssueID("oi_0123456789abcdef0123456789abcdef")
	verifier := "hmac:v"
	maxSends := otppolicy.V1().Terms().MaxSendsPerSubject

	usable := model.OTPChallenge{
		ContextVersion: "1",
		IssueID:        &issueID,
		Verifier:       &verifier,
		ExpiresAt:      &expires,
		NextIssueAt:    &next,
		AttemptCount:   0,
		SendCount:      1,
	}

	locked := usable
	locked.AttemptCount = 5

	tests := []struct {
		name            string
		challenge       model.OTPChallenge
		contextVersion  string
		terminal        bool
		businessBlocked bool
		want            string
	}{
		{
			name:           "terminal wins",
			challenge:      usable,
			contextVersion: "1",
			terminal:       true,
			want:           otp.AvailabilityNotApplicable,
		},
		{
			name:           "locked before business blocked",
			challenge:      locked,
			contextVersion: "1",
			want:           otp.AvailabilityLocked,
		},
		{
			name:            "business blocked even when crypto usable",
			challenge:       usable,
			contextVersion:  "1",
			businessBlocked: true,
			want:            otp.AvailabilityIssuanceBlocked,
		},
		{
			name:           "awaiting entry",
			challenge:      usable,
			contextVersion: "1",
			want:           otp.AvailabilityAwaitingEntry,
		},
		{
			name:           "stale context is not awaiting entry",
			challenge:      usable,
			contextVersion: "2",
			want:           otp.AvailabilityConsumedWait,
		},
		{
			name: "consumed wait",
			challenge: model.OTPChallenge{
				NextIssueAt: &next,
				SendCount:   1,
			},
			contextVersion: "1",
			want:           otp.AvailabilityConsumedWait,
		},
		{
			name:           "zero prior sends request available",
			challenge:      model.OTPChallenge{},
			contextVersion: "1",
			want:           otp.AvailabilityRequestAvailable,
		},
		{
			name: "third send still usable remains awaiting entry",
			challenge: model.OTPChallenge{
				ContextVersion: "1",
				IssueID:        &issueID,
				Verifier:       &verifier,
				ExpiresAt:      &expires,
				NextIssueAt:    &next,
				SendCount:      maxSends,
			},
			contextVersion: "1",
			want:           otp.AvailabilityAwaitingEntry,
		},
		{
			name: "consumed within window with exhausted send budget",
			challenge: model.OTPChallenge{
				NextIssueAt: &next,
				SendCount:   maxSends,
			},
			contextVersion: "1",
			want:           otp.AvailabilityConsumedWait,
		},
		{
			name: "expired challenge with remaining send budget",
			challenge: model.OTPChallenge{
				NextIssueAt: &pastNext,
				SendCount:   maxSends - 1,
			},
			contextVersion: "1",
			want:           otp.AvailabilityRequestAvailable,
		},
		{
			name: "send budget exhausted after issuance window ended",
			challenge: model.OTPChallenge{
				NextIssueAt: &pastNext,
				SendCount:   maxSends,
			},
			contextVersion: "1",
			want:           otp.AvailabilityIssuanceBlocked,
		},
		{
			name: "verification lockout with send budget exhausted",
			challenge: model.OTPChallenge{
				ContextVersion: "1",
				IssueID:        &issueID,
				Verifier:       &verifier,
				ExpiresAt:      &expires,
				NextIssueAt:    &pastNext,
				AttemptCount:   5,
				SendCount:      maxSends,
			},
			contextVersion: "1",
			want:           otp.AvailabilityLocked,
		},
		{
			name: "business blocker with send budget exhausted",
			challenge: model.OTPChallenge{
				NextIssueAt: &pastNext,
				SendCount:   maxSends,
			},
			contextVersion:  "1",
			businessBlocked: true,
			want:            otp.AvailabilityIssuanceBlocked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := otp.ProjectAvailability(
				tt.challenge,
				tt.contextVersion,
				now,
				5,
				maxSends,
				tt.terminal,
				tt.businessBlocked,
			)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAvailability_LoadsChallenge(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.request()
	require.NoError(t, err)

	out, err := h.uc.Availability(h.ctx, otp.AvailabilityInput{
		Purpose:        testPurpose,
		SubjectKey:     testSubjectKey,
		ContextVersion: testContextVersion,
	})
	require.NoError(t, err)
	require.True(t, out.Found)
	require.Equal(t, otp.AvailabilityAwaitingEntry, out.Availability)
	require.NotNil(t, out.OTPExpiresAt)

	blocked, err := h.uc.Availability(h.ctx, otp.AvailabilityInput{
		Purpose:         testPurpose,
		SubjectKey:      testSubjectKey,
		ContextVersion:  testContextVersion,
		BusinessBlocked: true,
	})
	require.NoError(t, err)
	require.Equal(t, otp.AvailabilityIssuanceBlocked, blocked.Availability)
}

func TestAvailability_SendBudgetExhaustedMatchesRequest(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	maxSends := otppolicy.V1().Terms().MaxSendsPerSubject

	for i := 0; i < maxSends; i++ {
		out, err := h.request()
		require.NoError(t, err)
		require.True(t, out.OTPSent)

		code := mustCodeFromSubmit(t, h.notification)
		_, err = h.verify(code)
		require.NoError(t, err)

		h.advance(2 * time.Minute)
	}

	out, err := h.uc.Availability(h.ctx, otp.AvailabilityInput{
		Purpose:        testPurpose,
		SubjectKey:     testSubjectKey,
		ContextVersion: testContextVersion,
	})
	require.NoError(t, err)
	require.Equal(t, otp.AvailabilityIssuanceBlocked, out.Availability)
	require.Equal(t, maxSends, out.Challenge.SendCount)

	_, err = h.request()
	requireMessageKey(t, err, "error.otp_send_limit_reached")

	// Polling must not mutate SendCount.
	again, err := h.uc.Availability(h.ctx, otp.AvailabilityInput{
		Purpose:        testPurpose,
		SubjectKey:     testSubjectKey,
		ContextVersion: testContextVersion,
	})
	require.NoError(t, err)
	require.Equal(t, maxSends, again.Challenge.SendCount)
	require.Equal(t, otp.AvailabilityIssuanceBlocked, again.Availability)
}
