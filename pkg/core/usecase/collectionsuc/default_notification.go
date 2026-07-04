package collectionsuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	notificationv1 "github.com/parstis/bnpl-backend/pkg/core/policy/defaultnotification/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// DefaultNoticeEligibility is a transaction-scoped, read-only eligibility view.
// Reserved prevents ordinary reminder dispatch before the default window or
// during ambiguous payment/cancellation reconciliation.
type DefaultNoticeEligibility = notificationv1.Eligibility

func (uc *UseCase) DefaultNoticeOnTx(ctx context.Context, tx repo.Tx, id model.CustomerID, now time.Time) (DefaultNoticeEligibility, error) {
	customerProfileQ := uc.customerProfile.Tx(tx)
	if _, err := customerProfileQ.LockByID(ctx, id); err != nil {
		return DefaultNoticeEligibility{}, err
	}

	episodeQ := uc.defaultEpisode.Tx(tx)
	episode, err := episodeQ.GetActiveByCustomerID(ctx, id)
	if err != nil {
		if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return DefaultNoticeEligibility{}, err
		}

		installmentQ := uc.installment.Tx(tx)
		evaluation, e := uc.evaluateObligations(ctx, installmentQ, id, now, false)
		return DefaultNoticeEligibility{Reserved: evaluation.IsDefaultEligible || evaluation.HasUnresolvedAdjustment}, e
	}

	result := DefaultNoticeEligibility{Reserved: true, EpisodeID: episode.ID}
	decisionQ := uc.decision.Tx(tx)
	opening, err := decisionQ.GetByID(ctx, episode.OpeningDecisionID)
	if err != nil {
		return result, err
	}

	policy, err := uc.policies.Resolve(opening.PolicyID, opening.PolicyVersion)
	if err != nil {
		return result, err
	}

	installmentQ := uc.installment.Tx(tx)
	evaluation, _, err := uc.evaluateObligationsWithFacts(ctx, installmentQ, id, now, true, policy)
	if err != nil {
		return result, err
	}

	triggers, err := episodeQ.ListTriggersByEpisodeID(ctx, episode.ID)
	if err != nil {
		return result, err
	}

	// Entry timing remains tied to immutable episode history. Message references
	// current qualifying debt, never an already-paid original trigger. If only
	// within-grace due debt remains, keep the episode but suppress this notice.
	result.InstallmentIDs = append(result.InstallmentIDs, evaluation.TriggerInstallmentIDs...)
	var boundary time.Time
	for _, trigger := range triggers {
		if boundary.IsZero() || trigger.OverdueEligibleAt.Before(boundary) {
			boundary = trigger.OverdueEligibleAt
		}
	}

	result.Eligible = notificationv1.Eligible(now, boundary, uc.clock.BusinessLocation(), !evaluation.IsRecovered, len(result.InstallmentIDs) > 0, evaluation.HasPaymentUncertainty)
	return result, nil
}
