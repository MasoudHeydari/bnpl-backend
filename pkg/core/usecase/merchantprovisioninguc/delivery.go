package merchantprovisioninguc

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// DeliverDue claims one pending report per iteration. Leases recover work after
// a process crash; a generation check prevents an expired worker overwriting a
// later delivery. Remote retries always use the original immutable report.
func (uc *UseCase) DeliverDue(ctx context.Context) error {
	const op = "merchantprovisioninguc.DeliverDue"

	for range 32 {
		if err := ctx.Err(); err != nil {
			return err
		}

		var p model.MerchantParticipation
		now := uc.clock.Now()
		err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
				participationQ := uc.participation.Tx(tx)
				var err error
				p, err = participationQ.ClaimDue(ctx, now, now.Add(time.Minute))
				return err
			})
		})
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("%s: claim progress: %w", op, err)
		}

		progress := merchant.Progress{
			RequestID:           p.RequestID,
			EventID:             p.EventID,
			OwnerVersion:        p.OwnerVersion,
			DeclarationRevision: p.DeclarationRevision,
			Status:              p.ProgressStatus,
			Issues:              p.Issues,
		}
		if p.ProfileID != nil {
			progress.AccountReference = p.ProfileID.String()
		}

		sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		snapshot, sendErr := uc.client.ReportProgress(sendCtx, progress)
		cancel()
		now = uc.clock.Now()
		result := repo.MerchantProgressDeliveryResult{
			RequestID:           p.RequestID,
			DeclarationRevision: p.DeclarationRevision,
			AttemptCount:        p.AttemptCount,
			Status:              "delivered",
			NextAttemptAt:       now,
			Now:                 now,
		}
		switch {
		case errors.Is(sendErr, merchant.ErrDeclarationRevisionConflict):
			// A new declaration proves the preceding CHANGES_REQUESTED report
			// was observed only if Merchant's version is at least ours.
			readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			ref, err := uc.client.GetParticipation(readCtx, p.RequestID)
			cancel()
			if err != nil {
				result.Status, result.Reason = "pending", "merchant_reconciliation_unavailable"
				result.NextAttemptAt = now.Add(retryDelay(p.AttemptCount))
				break
			}
			if ref.RequestID != p.RequestID ||
				ref.MerchantID != p.MerchantID ||
				ref.ApplicationID != p.ApplicationID ||
				ref.Environment != p.Environment ||
				ref.BusinessLine != "bnpl" ||
				ref.DeclarationRevision <= p.DeclarationRevision ||
				ref.OwnerVersion < p.OwnerVersion ||
				p.ProgressStatus != model.MerchantParticipationChangesRequested {
				result.Status, result.Reason = "blocked", "merchant_declaration_revision_conflict"
			}

		case errors.Is(sendErr, merchant.ErrBlocked), errors.Is(sendErr, merchant.ErrReferenceNotFound):
			result.Status, result.Reason = "blocked", "merchant_contract_or_access_error"

		case sendErr != nil:
			result.Status, result.Reason = "pending", "merchant_temporary_failure"
			result.NextAttemptAt = now.Add(retryDelay(p.AttemptCount))

		case snapshot.RequestID != p.RequestID ||
			snapshot.BusinessLine != "bnpl" ||
			snapshot.Environment != p.Environment ||
			snapshot.OwnerVersion < p.OwnerVersion ||
			snapshot.DeclarationRevision < p.DeclarationRevision:
			result.Status, result.Reason = "blocked", "merchant_projection_mismatch"

		case snapshot.OwnerVersion == p.OwnerVersion && snapshot.DeclarationRevision == p.DeclarationRevision &&
			(snapshot.Status != progress.Status || snapshot.AccountReference != progress.AccountReference || !slices.Equal(snapshot.Issues, progress.Issues)):
			result.Status, result.Reason = "blocked", "merchant_projection_mismatch"
		}

		if result.Status == "pending" && p.AttemptCount >= 12 {
			result.Status, result.Reason = "blocked", "merchant_retry_exhausted"
		}

		err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
				participationQ := uc.participation.Tx(tx)
				return participationQ.Finish(ctx, result)
			})
		})
		if err != nil {
			return fmt.Errorf("%s: persist delivery outcome: %w", op, err)
		}

		if result.Status != "delivered" {
			uc.logger.With("participation_request_id", p.RequestID.String(), "reason", result.Reason).Warn(log.Msg("merchant progress delivery requires retry or correction"))
		}
	}

	return nil
}

func retryDelay(attempt int64) time.Duration {
	// Saturate before shifting to avoid overflow on long outages.
	exponent := max(int64(0), min(attempt-1, int64(8)))
	delay := time.Duration(1<<exponent) * time.Second
	return delay/2 + time.Duration(rand.Int64N(int64(delay/2)))
}
