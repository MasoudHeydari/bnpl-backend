package merchantprovisioninguc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/merchantprovisioning"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Process evaluates the current declaration and records one immutable outcome.
// A completed event or revision does not need a live Merchant dependency.
func (uc *UseCase) Process(ctx context.Context, input Input) error {
	const op = "merchantprovisioninguc.Process"

	if strings.TrimSpace(input.Environment) == "" ||
		input.MerchantID.IsZero() ||
		input.ApplicationID.IsZero() ||
		input.RequestID.IsZero() ||
		input.DeclarationRevision < 0 ||
		input.Event.EventID == "" ||
		input.Event.PayloadFingerprint == "" {
		return ErrInvalidReference
	}

	var alreadyProcessed, evaluated bool
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

		evaluated = p.DeclarationRevision >= input.DeclarationRevision
		return nil
	})
	if err != nil {
		return fmt.Errorf("%s: read durable identities: %w", op, err)
	}

	if alreadyProcessed {
		return nil
	}

	var ref merchant.Reference
	var terms merchantprovisioning.Terms
	var skip bool
	issues := make([]model.MerchantParticipationIssue, 0)
	if !evaluated {
		ref, err = uc.client.GetParticipation(ctx, input.RequestID)
		if err != nil {
			if errors.Is(err, merchant.ErrReferenceNotFound) {
				return ErrInvalidReference
			}

			return fmt.Errorf("%s: read participation reference: %w", op, err)
		}

		if ref.MerchantID != input.MerchantID ||
			ref.ApplicationID != input.ApplicationID ||
			ref.RequestID != input.RequestID ||
			ref.BusinessLine != "bnpl" ||
			ref.Environment != input.Environment {
			return ErrInvalidReference
		}

		switch {
		case ref.DeclarationRevision < input.DeclarationRevision:
			return fmt.Errorf("%s: declaration snapshot is behind the event", op)

		case ref.DeclarationRevision > input.DeclarationRevision:
			skip = true

		case ref.BaseApplicationStatus != "APPROVED" || ref.Status == "REJECTED" || ref.Status == "SUSPENDED":
			skip = true

		case ref.Status != "REQUESTED":
			// READY without a corresponding local durable evaluation is an
			// operational conflict. Never reconstruct accounts from a remote status.
			return ErrConflict

		case ref.DeclarationRevision == 0 || ref.SelfDeclaration == nil:
			issues = append(issues, model.MerchantParticipationIssue{
				Field:   "self_declaration",
				Code:    "bnpl.declaration_required",
				Message: "Complete the BNPL self declaration.",
			})

		default:
			terms, err = uc.policy.Evaluate(*ref.SelfDeclaration)
			if err != nil {
				if !errors.Is(err, merchantprovisioning.ErrInvalidDeclaration) {
					return fmt.Errorf("%s: evaluate merchant declaration: %w", op, err)
				}

				issues = append(issues, model.MerchantParticipationIssue{
					Field:   "self_declaration",
					Code:    "bnpl.invalid_self_declaration",
					Message: "Declared amount, currency or tenure does not satisfy BNPL policy.",
				})
			}
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

			current, err := participationQ.Get(ctx, input.RequestID)
			exists := err == nil
			if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
				return err
			}

			if exists && !current.MatchesCommand(input.MerchantID, input.ApplicationID, input.Environment) {
				return ErrConflict
			}

			if !skip && (!exists || current.DeclarationRevision < input.DeclarationRevision) {
				if evaluated {
					return fmt.Errorf("%s: durable evaluation disappeared", op)
				}

				// A fresh revision may follow a previously delivered rejection,
				// never a READY account. Merchant cannot assign a new BNPL version.
				if exists && current.ProgressStatus != model.MerchantParticipationChangesRequested {
					return ErrConflict
				}

				if ref.OwnerVersion != current.OwnerVersion || ref.OwnerVersion == math.MaxInt64 {
					return ErrConflict
				}

				p := model.MerchantParticipation{
					RequestID:           input.RequestID,
					ApplicationID:       input.ApplicationID,
					MerchantID:          input.MerchantID,
					Environment:         input.Environment,
					DeclarationRevision: input.DeclarationRevision,
					EventID:             uc.ids.Generate(),
					OwnerVersion:        ref.OwnerVersion + 1,
					PolicyID:            uc.policy.Ref().ID,
					PolicyVersion:       uc.policy.Ref().Version,
					Issues:              issues,
					ProgressStatus:      model.MerchantParticipationChangesRequested,
				}
				if len(issues) == 0 {
					if strings.TrimSpace(ref.TradeName) == "" {
						return ErrInvalidReference
					}

					profile, err := uc.ensureProfile(
						ctx,
						merchantProfileQ,
						input.MerchantID,
						ref.TradeName,
						terms,
						now,
					)
					if err != nil {
						return err
					}

					p.ProfileID = &profile.ID
					p.ProgressStatus = model.MerchantParticipationReady
				}

				if err := participationQ.Insert(ctx, p, now); err != nil {
					// A concurrent evaluation can win a unique constraint. Roll
					// back and retry against the committed result.
					return err
				}
			}

			_, err = inboxQ.MarkProcessed(ctx, inbox.ID, now, now)
			if err != nil {
				return err
			}

			return nil
		})
	})
	if err != nil {
		return fmt.Errorf("%s: persist evaluation and report: %w", op, err)
	}

	return nil
}

func (uc *UseCase) ensureProfile(
	ctx context.Context,
	merchantProfileQ repo.MerchantProfileTxQueryer,
	merchantID model.ExternalMerchantID,
	tradeName string,
	terms merchantprovisioning.Terms,
	now time.Time,
) (model.MerchantProfile, error) {
	profile, err := merchantProfileQ.LockByMerchantID(ctx, merchantID)
	if err == nil {
		// Do not reactivate or change the policy of an existing account on replay.
		if profile.Status != model.ActiveMerchantStatus {
			return model.MerchantProfile{}, ErrInvalidReference
		}

		if profile.MaxAmountMinor != terms.Limits.MaxAmountMinor ||
			profile.MaxAmountCurrency != terms.Limits.MaxAmountCurrency ||
			profile.MaxTenureMonths != terms.Limits.MaxTenureMonths {
			return model.MerchantProfile{}, ErrConflict
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
		terms.FundingModels,
		terms.SettlementModels,
		terms.Currencies,
		terms.RiskTier,
		terms.Limits.MaxAmountMinor,
		terms.Limits.MaxAmountCurrency,
		terms.Limits.MaxTenureMonths,
		terms.RequiresBranchMode,
		terms.Jurisdiction,
		now,
		now,
	)
	if err != nil {
		return model.MerchantProfile{}, err
	}

	for _, kind := range terms.CreditKinds {
		err = merchantProfileQ.SetCreditKindEnabled(
			ctx,
			profile.ID,
			kind,
			true,
			now,
		)
		if err != nil {
			return model.MerchantProfile{}, err
		}
	}

	return profile, nil
}
