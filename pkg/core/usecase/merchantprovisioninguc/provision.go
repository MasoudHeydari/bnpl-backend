package merchantprovisioninguc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Process provisions a configured account, never a terminal, outlet or credential.
// An already completed event/request does not need a live Merchant dependency.
func (uc *UseCase) Process(ctx context.Context, input Input) error {
	const op = "merchantprovisioninguc.Process"

	if input.Environment != "SANDBOX" || input.MerchantID.IsZero() || input.ApplicationID.IsZero() || input.RequestID.IsZero() || input.Event.EventID == "" || input.Event.PayloadFingerprint == "" {
		return ErrInvalidReference
	}

	var alreadyProcessed, existingRequest bool
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		inboxQ := uc.inbox.Conn(conn)
		participationQ := uc.participation.Conn(conn)
		row, err := inboxQ.GetBySourceAndEventID(ctx, input.Event.SourceService, input.Event.EventID)
		if err == nil {
			if row.PayloadFingerprint != input.Event.PayloadFingerprint {
				return ErrConflict
			}

			alreadyProcessed = row.Status == model.ProcessedInboxStatus
			if alreadyProcessed {
				return nil
			}
		} else if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return err
		}

		p, err := participationQ.Get(ctx, input.RequestID)
		if err != nil {
			if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				return nil
			}

			return err
		}

		if !p.MatchesCommand(input.MerchantID, input.ApplicationID, input.Environment) {
			return ErrConflict
		}

		existingRequest = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("%s: read durable identities: %w", op, err)
	}

	if alreadyProcessed {
		return nil
	}

	var ref merchant.Reference
	if !existingRequest {
		ref, err = uc.client.GetParticipation(ctx, input.RequestID)
		if err != nil {
			if errors.Is(err, merchant.ErrReferenceNotFound) {
				return ErrInvalidReference
			}

			return fmt.Errorf("%s: read participation reference: %w", op, err)
		}

		if ref.MerchantID != input.MerchantID || ref.RequestID != input.RequestID || ref.BusinessLine != "bnpl" || ref.Environment != input.Environment || ref.BaseApplicationStatus != "APPROVED" || strings.TrimSpace(ref.TradeName) == "" {
			return ErrInvalidReference
		}
	}

	now := uc.clock.Now()
	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			inboxQ := uc.inbox.Tx(tx)
			participationQ := uc.participation.Tx(tx)
			merchantProfileQ := uc.merchantProfile.Tx(tx)
			inbox, acquired, err := inboxQ.AcquireForProcessing(ctx, input.Event, now)
			if err != nil {
				return err
			}

			switch acquired {
			case model.InboxAcquireAlreadyProcessed:
				return nil

			case model.InboxAcquireNew:
				// Continue with the immutable request identity.

			default:
				return ErrConflict
			}

			p, err := participationQ.Get(ctx, input.RequestID)
			if err == nil {
				if !p.MatchesCommand(input.MerchantID, input.ApplicationID, input.Environment) {
					return ErrConflict
				}
			} else if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				if existingRequest {
					return fmt.Errorf("%s: durable request disappeared", op)
				}

				profile, err := uc.ensureProfile(ctx, merchantProfileQ, input.MerchantID, ref.TradeName, now)
				if err != nil {
					return err
				}

				p = model.MerchantParticipation{
					RequestID: input.RequestID, ApplicationID: input.ApplicationID,
					MerchantID: input.MerchantID, ProfileID: profile.ID, Environment: input.Environment,
					EventID: uc.ids.Generate(), OwnerVersion: 1,
				}
				err = participationQ.Insert(ctx, p, now)
				if err != nil {
					// A concurrent command can win a unique constraint. Roll back and let
					// the consumer retry against the winning committed request/account.
					return err
				}
			} else {
				return err
			}

			_, err = inboxQ.MarkProcessed(ctx, inbox.ID, now, now)
			if err != nil {
				return err
			}

			return nil
		})
	})
	if err != nil {
		return fmt.Errorf("%s: persist account and READY report: %w", op, err)
	}

	return nil
}

func (uc *UseCase) ensureProfile(
	ctx context.Context,
	merchantProfileQ repo.MerchantProfileTxQueryer,
	merchantID model.ExternalMerchantID,
	tradeName string,
	now time.Time,
) (model.MerchantProfile, error) {
	profile, err := merchantProfileQ.LockByMerchantID(ctx, merchantID)
	if err == nil {
		// Do not reactivate or change the policy of an existing account on replay.
		if profile.Status != model.ActiveMerchantStatus {
			return model.MerchantProfile{}, ErrInvalidReference
		}

		return profile, nil
	}

	if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.MerchantProfile{}, err
	}

	// DisplayName is the existing local projection for Hosted Checkout/OTP.
	// Never substitute the trade name for legal identity or invent other public data.
	profile, err = merchantProfileQ.Create(
		ctx,
		merchantID,
		tradeName,
		"",
		"",
		"",
		"",
		model.ActiveMerchantStatus,
		[]model.FundingModel{model.PlatformFundedFundingModel},
		[]model.SettlementModel{model.DirectSettlementModel},
		[]model.Currency{model.IRRCurrency},
		model.LowRiskTier,
		defaultMerchantMaxAmountMinor,
		model.IRRCurrency,
		defaultMerchantMaxTenureMonths,
		false,
		model.IRJurisdiction,
		now,
		now,
	)
	if err != nil {
		return model.MerchantProfile{}, err
	}

	err = merchantProfileQ.SetCreditKindEnabled(ctx, profile.ID, model.PayInInstallmentsCreditKind, true, now)
	if err != nil {
		return model.MerchantProfile{}, err
	}

	return profile, nil
}
