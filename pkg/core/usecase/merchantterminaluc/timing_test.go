package merchantterminaluc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestRotateNaturalExpirationUsesRetireAtAsRetiredAt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()

	grace := time.Hour
	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &grace))
	require.NoError(t, err)

	var retiring model.MerchantTerminalCredential
	for _, credential := range h.terminals.credentials {
		if credential.ID == first.CredentialID {
			retiring = credential
			break
		}
	}

	require.Equal(t, model.RetiringMerchantTerminalCredentialStatus, retiring.Status)
	require.NotNil(t, retiring.RetireAt)
	retireAt := *retiring.RetireAt

	h.advance(2 * time.Hour)

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "rotate-2", nil))
	require.NoError(t, err)

	for _, credential := range h.terminals.credentials {
		if credential.ID == first.CredentialID {
			require.Equal(t, model.RetiredMerchantTerminalCredentialStatus, credential.Status)
			require.NotNil(t, credential.RetiredAt)
			require.Equal(t, retireAt, *credential.RetiredAt)
			require.Equal(t, h.now, credential.UpdatedAt)

			return
		}
	}

	t.Fatal("retired credential not found")
}

func TestRotateZeroGraceRetiresAtRotationTime(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()
	zero := time.Duration(0)

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &zero))
	require.NoError(t, err)

	for _, credential := range h.terminals.credentials {
		if credential.ID == first.CredentialID {
			require.Equal(t, model.RetiredMerchantTerminalCredentialStatus, credential.Status)
			require.NotNil(t, credential.RetiredAt)
			require.Equal(t, h.now, *credential.RetiredAt)

			return
		}
	}

	t.Fatal("retired credential not found")
}

func TestRotateReplayOmitsExpiredRetiringCredential(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	// Grace must expire inside the issuance replay window so the response can
	// be rebuilt without a DB status write.
	grace := 5 * time.Minute
	firstRotate, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &grace))
	require.NoError(t, err)
	require.NotNil(t, firstRotate.Retiring)

	// Leave the DB row as RETIRING but advance past retire_at without rotating again.
	h.advance(6 * time.Minute)

	replayed, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &grace))
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	require.Equal(t, firstRotate.APIKey, replayed.APIKey)
	require.Nil(t, replayed.Retiring, "expired retiring credential must not appear in replay")
}

func TestManualRetireUsesNowAsRetiredAt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()

	grace := time.Hour
	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &grace))
	require.NoError(t, err)

	h.advance(5 * time.Minute)

	out, err := h.uc.Retire(h.ctx, RetireInput{
		MerchantID:   h.merchantID,
		CredentialID: first.CredentialID,
	})
	require.NoError(t, err)
	require.Equal(t, h.now, out.RetiredAt)
}
