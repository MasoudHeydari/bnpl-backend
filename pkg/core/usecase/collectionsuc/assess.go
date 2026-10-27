package collectionsuc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	collectionsv1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// AssessBatchOutput reports one periodic collections assessment run.
type AssessBatchOutput struct {
	AssessedCount  int
	DefaultedCount int
	RecoveredCount int
	SkippedCount   int
	FailedCount    int
}

// AssessCustomerOutput reports the assessment outcome for one customer.
type AssessCustomerOutput struct {
	CustomerID            model.CustomerID
	Transition            AssessTransition
	EpisodeID             *model.DefaultEpisodeID
	OpeningDecisionID     *model.DecisionID
	ClosingDecisionID     *model.DecisionID
	HasPaymentUncertainty bool
}

// AssessTransition classifies the durable outcome of one customer assessment.
type AssessTransition string

// Known assessment transitions.
const (
	AssessTransitionNone      AssessTransition = "none"
	AssessTransitionDefaulted AssessTransition = "defaulted"
	AssessTransitionRecovered AssessTransition = "recovered"
	AssessTransitionBlocked   AssessTransition = "blocked"
)

// AssessBatch evaluates collections state for the next keyset page of candidates.
func (uc *UseCase) AssessBatch(ctx context.Context) (AssessBatchOutput, error) {
	const op = "collectionsuc.UseCase.AssessBatch"

	started := uc.clock.Now().UTC()
	var output AssessBatchOutput
	afterCustomerID := model.CustomerID{}

	for {
		var customerIDs []model.CustomerID

		err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			installmentQ := uc.installment.Conn(conn)
			rows, listErr := installmentQ.ListCollectionsAssessmentCustomerPage(
				ctx,
				started,
				afterCustomerID,
				uc.batchSize,
			)
			if listErr != nil {
				return listErr
			}

			customerIDs = rows
			return nil
		})
		if err != nil {
			uc.businessMetrics.CollectionsAssessmentJobCompleted(ctx, "failure")
			uc.businessMetrics.CollectionsAssessmentJobDuration(ctx, uc.clock.Now().UTC().Sub(started))
			return AssessBatchOutput{}, fmt.Errorf("%s: list assessment candidates: %w", op, err)
		}

		if len(customerIDs) == 0 {
			break
		}

		for _, customerID := range customerIDs {
			result, assessErr := uc.AssessCustomer(ctx, customerID)
			output.AssessedCount++

			if assessErr != nil {
				output.FailedCount++
				continue
			}

			switch result.Transition {
			case AssessTransitionDefaulted:
				output.DefaultedCount++

			case AssessTransitionRecovered:
				output.RecoveredCount++

			default:
				output.SkippedCount++
			}
		}

		afterCustomerID = customerIDs[len(customerIDs)-1]
		if len(customerIDs) < uc.batchSize {
			break
		}
	}

	uc.businessMetrics.CollectionsAssessmentJobCompleted(ctx, "success")
	uc.businessMetrics.CollectionsAssessmentJobDuration(ctx, uc.clock.Now().UTC().Sub(started))
	uc.businessMetrics.CollectionsAssessmentCustomersObserved(ctx, output.AssessedCount)
	uc.businessMetrics.CollectionsDefaulted(ctx, output.DefaultedCount)
	uc.businessMetrics.CollectionsRecovered(ctx, output.RecoveredCount)
	uc.businessMetrics.CollectionsAssessmentSkipped(ctx, output.SkippedCount)

	return output, nil
}

// AssessCustomer evaluates and, when needed, persists default entry or recovery
// for one customer inside a single transaction.
func (uc *UseCase) AssessCustomer(
	ctx context.Context,
	customerID model.CustomerID,
) (AssessCustomerOutput, error) {
	const op = "collectionsuc.UseCase.AssessCustomer"

	if customerID.IsZero() {
		return AssessCustomerOutput{}, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	now := uc.clock.Now().UTC()
	var output AssessCustomerOutput
	output.CustomerID = customerID

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			result, assessErr := uc.assessCustomerTx(ctx, tx, customerID, now)
			if assessErr != nil {
				return assessErr
			}

			output = result
			return nil
		})
	})
	if err != nil {
		return AssessCustomerOutput{}, err
	}

	return output, nil
}

// AssertNewCreditAllowed re-assesses collections state under the caller's
// transaction and returns a semantic conflict when new credit usage is blocked.
//
// Call only after the customer profile row is locked. Fail-closed against the
// live default rule even when the restriction row is not yet materialized by
// ensuring assessment runs first.
func (uc *UseCase) AssertNewCreditAllowed(
	ctx context.Context,
	tx repo.Tx,
	customerID model.CustomerID,
	now time.Time,
) error {
	const op = "collectionsuc.UseCase.AssertNewCreditAllowed"

	if customerID.IsZero() {
		return cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	result, err := uc.assessCustomerTx(ctx, tx, customerID, now.UTC())
	if err != nil {
		return fmt.Errorf("%s: assess customer: %w", op, err)
	}

	restrictionQ := uc.customerRestriction.Tx(tx)
	_, err = restrictionQ.GetActiveByCustomerType(
		ctx,
		customerID,
		model.NewCreditUsageCustomerRestrictionType,
	)
	if err == nil {
		return newCreditUsageRestrictedError()
	}

	if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return fmt.Errorf("%s: load active restriction: %w", op, err)
	}

	if result.Transition == AssessTransitionDefaulted || result.Transition == AssessTransitionBlocked {
		return newCreditUsageRestrictedError()
	}

	// Live default eligibility without a successful materialization is still blocked.
	episodeQ := uc.defaultEpisode.Tx(tx)
	_, episodeErr := episodeQ.GetActiveByCustomerID(ctx, customerID)
	if episodeErr == nil {
		return newCreditUsageRestrictedError()
	}

	if !cerr.IsCode(episodeErr, cerr.CodeResourceNotFound) {
		return fmt.Errorf("%s: load active episode: %w", op, episodeErr)
	}

	return nil
}

func (uc *UseCase) assessCustomerTx(
	ctx context.Context,
	tx repo.Tx,
	customerID model.CustomerID,
	now time.Time,
) (AssessCustomerOutput, error) {
	const op = "collectionsuc.UseCase.assessCustomerTx"

	customerProfileQ := uc.customerProfile.Tx(tx)
	installmentQ := uc.installment.Tx(tx)
	decisionQ := uc.decision.Tx(tx)
	episodeQ := uc.defaultEpisode.Tx(tx)
	restrictionQ := uc.customerRestriction.Tx(tx)
	caseQ := uc.collectionsCase.Tx(tx)
	outboxQ := uc.outbox.Tx(tx)

	profile, err := customerProfileQ.LockByID(ctx, customerID)
	if err != nil {
		return AssessCustomerOutput{}, fmt.Errorf("%s: lock customer profile: %w", op, err)
	}

	obligations, err := installmentQ.ListCollectionsObligationsByCustomerID(ctx, customerID)
	if err != nil {
		return AssessCustomerOutput{}, fmt.Errorf("%s: list obligations: %w", op, err)
	}

	facts := make([]collectionsv1.ObligationFact, 0, len(obligations))
	factByID := make(map[model.InstallmentID]repo.CollectionsObligation, len(obligations))
	for _, row := range obligations {
		factByID[row.Installment.ID] = row
		facts = append(facts, collectionsv1.ObligationFact{
			InstallmentID:             row.Installment.ID,
			ContractID:                row.Installment.ContractID,
			DueAt:                     row.Installment.DueAt,
			GracePeriodDays:           row.Installment.GracePeriodDays,
			Status:                    row.Installment.Status,
			IsFullySettled:            false,
			HasUnresolvedPayment:      row.HasUnresolvedPayment,
			IsContractuallyApplicable: true,
		})
	}

	activeEpisode, episodeErr := episodeQ.LockActiveByCustomerID(ctx, customerID)
	currentlyDefaulted := false
	if episodeErr == nil {
		currentlyDefaulted = true
	} else if !cerr.IsCode(episodeErr, cerr.CodeResourceNotFound) {
		return AssessCustomerOutput{}, fmt.Errorf("%s: lock active episode: %w", op, episodeErr)
	}

	evaluation := uc.policy.Evaluate(facts, now, currentlyDefaulted)

	output := AssessCustomerOutput{
		CustomerID:            customerID,
		Transition:            AssessTransitionNone,
		HasPaymentUncertainty: evaluation.HasPaymentUncertainty,
	}

	if currentlyDefaulted {
		episodeID := activeEpisode.ID
		output.EpisodeID = &episodeID
		openingID := activeEpisode.OpeningDecisionID
		output.OpeningDecisionID = &openingID

		if !evaluation.IsRecovered {
			output.Transition = AssessTransitionBlocked
			if evaluation.HasPaymentUncertainty {
				if noteErr := uc.notePaymentUncertainty(ctx, caseQ, activeEpisode, now); noteErr != nil {
					return AssessCustomerOutput{}, noteErr
				}
			}

			return output, nil
		}

		recovered, recoverErr := uc.recoverEpisode(
			ctx,
			decisionQ,
			episodeQ,
			restrictionQ,
			caseQ,
			outboxQ,
			profile,
			activeEpisode,
			now,
		)
		if recoverErr != nil {
			return AssessCustomerOutput{}, recoverErr
		}

		output.Transition = AssessTransitionRecovered
		output.ClosingDecisionID = &recovered.ClosingDecisionID
		return output, nil
	}

	if !evaluation.IsDefaultEligible {
		return output, nil
	}

	opened, openErr := uc.openEpisode(
		ctx,
		decisionQ,
		episodeQ,
		restrictionQ,
		caseQ,
		outboxQ,
		profile,
		evaluation,
		factByID,
		now,
	)
	if openErr != nil {
		return AssessCustomerOutput{}, openErr
	}

	output.Transition = AssessTransitionDefaulted
	output.EpisodeID = &opened.EpisodeID
	output.OpeningDecisionID = &opened.OpeningDecisionID
	output.HasPaymentUncertainty = evaluation.HasPaymentUncertainty
	return output, nil
}

type openedEpisode struct {
	EpisodeID         model.DefaultEpisodeID
	OpeningDecisionID model.DecisionID
}

type recoveredEpisode struct {
	ClosingDecisionID model.DecisionID
}

func (uc *UseCase) openEpisode(
	ctx context.Context,
	decisionQ repo.DecisionTxQueryer,
	episodeQ repo.DefaultEpisodeTxQueryer,
	restrictionQ repo.CustomerRestrictionTxQueryer,
	caseQ repo.CollectionsCaseTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	profile model.CustomerProfile,
	evaluation collectionsv1.Evaluation,
	factByID map[model.InstallmentID]repo.CollectionsObligation,
	now time.Time,
) (openedEpisode, error) {
	const op = "collectionsuc.UseCase.openEpisode"

	// Fresh UUID keeps concurrent open attempts from sharing a Decision key;
	// the active-episode unique index still collapses races to one episode and
	// rolls back the losing transaction atomically.
	idempotencyKey := "collections_default_open:" + uuid.NewString()
	fingerprint := fmt.Sprintf(
		"%s|%s|%s|%v",
		collectionsv1.PolicyID,
		collectionsv1.PolicyVersion,
		profile.ID.String(),
		evaluation.TriggerInstallmentIDs,
	)

	decision, err := decisionQ.Create(
		ctx,
		model.RestrictionDecisionType,
		model.CustomerProfileDecisionTargetType,
		profile.ID.UUID(),
		model.RestrictedDecisionOutcome,
		[]string{model.CollectionsGracePeriodExceededDecisionReason},
		collectionsv1.PolicyID,
		collectionsv1.PolicyVersion,
		profile.Jurisdiction,
		model.SystemDecisionActorType,
		nil,
		nil,
		nil,
		nil,
		now,
		now,
		idempotencyKey,
		fingerprint,
	)
	if err != nil {
		return openedEpisode{}, fmt.Errorf("%s: create restriction decision: %w", op, err)
	}

	episodeID := model.DefaultEpisodeID(uuid.New())
	triggers := make([]model.DefaultEpisodeTrigger, 0, len(evaluation.TriggerInstallmentIDs))
	for _, installmentID := range evaluation.TriggerInstallmentIDs {
		row, ok := factByID[installmentID]
		if !ok {
			return openedEpisode{}, cerr.InternalError(fmt.Errorf(
				"%s: missing trigger installment %s",
				op,
				installmentID,
			))
		}

		triggers = append(triggers, model.DefaultEpisodeTrigger{
			EpisodeID:         episodeID,
			InstallmentID:     installmentID,
			DueAt:             row.Installment.DueAt,
			GracePeriodDays:   row.Installment.GracePeriodDays,
			OverdueEligibleAt: row.Installment.OverdueEligibleAt(),
		})
	}

	episode, err := episodeQ.Create(ctx, repo.CreateDefaultEpisode{
		ID:                episodeID,
		CustomerID:        profile.ID,
		OpeningDecisionID: decision.ID,
		StartedAt:         now,
		CreatedAt:         now,
		UpdatedAt:         now,
		Triggers:          triggers,
	})
	if err != nil {
		// Unique violations abort the PostgreSQL transaction. Callers retry after
		// the winning assessment commits; customer LockByID serializes normal paths.
		return openedEpisode{}, fmt.Errorf("%s: create default episode: %w", op, err)
	}

	episodeIDPtr := episode.ID
	_, err = restrictionQ.Create(ctx, repo.CreateCustomerRestriction{
		ID:                model.CustomerRestrictionID(uuid.New()),
		CustomerID:        profile.ID,
		RestrictionType:   model.NewCreditUsageCustomerRestrictionType,
		Reason:            model.CollectionsDefaultCustomerRestrictionReason,
		DefaultEpisodeID:  &episodeIDPtr,
		OpeningDecisionID: decision.ID,
		AppliedAt:         now,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		return openedEpisode{}, fmt.Errorf("%s: create customer restriction: %w", op, err)
	}

	caseRow, err := caseQ.Create(ctx, repo.CreateCollectionsCase{
		ID:                model.CollectionsCaseID(uuid.New()),
		CustomerID:        profile.ID,
		DefaultEpisodeID:  episode.ID,
		OpeningDecisionID: decision.ID,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		return openedEpisode{}, fmt.Errorf("%s: create collections case: %w", op, err)
	}

	openedPayload, _ := json.Marshal(map[string]any{
		"customer_id":             profile.ID.String(),
		"default_episode_id":      episode.ID.String(),
		"opening_decision_id":     decision.ID.String(),
		"trigger_count":           len(triggers),
		"has_payment_uncertainty": evaluation.HasPaymentUncertainty,
	})

	_, err = caseQ.AppendEvent(ctx, repo.CreateCollectionsCaseEvent{
		ID:         model.CollectionsCaseEventID(uuid.New()),
		CaseID:     caseRow.ID,
		EventType:  model.CaseOpenedCollectionsCaseEventType,
		DecisionID: &decision.ID,
		Payload:    openedPayload,
		OccurredAt: now,
		ActorType:  model.SystemDecisionActorType,
		ActorID:    nil,
		CreatedAt:  now,
	})
	if err != nil {
		return openedEpisode{}, fmt.Errorf("%s: append case opened event: %w", op, err)
	}

	if evaluation.HasPaymentUncertainty {
		uncertaintyPayload, _ := json.Marshal(map[string]any{
			"customer_id":        profile.ID.String(),
			"default_episode_id": episode.ID.String(),
		})
		_, err = caseQ.AppendEvent(ctx, repo.CreateCollectionsCaseEvent{
			ID:         model.CollectionsCaseEventID(uuid.New()),
			CaseID:     caseRow.ID,
			EventType:  model.PaymentUncertaintyNotedCollectionsCaseEventType,
			DecisionID: &decision.ID,
			Payload:    uncertaintyPayload,
			OccurredAt: now,
			ActorType:  model.SystemDecisionActorType,
			CreatedAt:  now,
		})
		if err != nil {
			return openedEpisode{}, fmt.Errorf("%s: append payment uncertainty event: %w", op, err)
		}
	}

	if err := uc.insertCollectionsAuditOutbox(
		ctx,
		outboxQ,
		"collections.default_episode.open",
		"default_episode",
		episode.ID.String(),
		decision.ID,
		profile.ID,
		now,
		map[string]any{
			"policy_id":      collectionsv1.PolicyID,
			"policy_version": collectionsv1.PolicyVersion,
			"reason_code":    model.CollectionsGracePeriodExceededDecisionReason,
			"case_id":        caseRow.ID.String(),
		},
	); err != nil {
		return openedEpisode{}, err
	}

	return openedEpisode{
		EpisodeID:         episode.ID,
		OpeningDecisionID: decision.ID,
	}, nil
}

func (uc *UseCase) recoverEpisode(
	ctx context.Context,
	decisionQ repo.DecisionTxQueryer,
	episodeQ repo.DefaultEpisodeTxQueryer,
	restrictionQ repo.CustomerRestrictionTxQueryer,
	caseQ repo.CollectionsCaseTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	profile model.CustomerProfile,
	episode model.DefaultEpisode,
	now time.Time,
) (recoveredEpisode, error) {
	const op = "collectionsuc.UseCase.recoverEpisode"

	priorDecisionID := episode.OpeningDecisionID
	// One recovery Decision chain per episode identity.
	idempotencyKey := "collections_default_recover:" + episode.ID.String()
	fingerprint := fmt.Sprintf(
		"%s|%s|recover|%s|%s",
		collectionsv1.PolicyID,
		collectionsv1.PolicyVersion,
		episode.ID.String(),
		priorDecisionID.String(),
	)

	decision, err := decisionQ.Create(
		ctx,
		model.RestrictionDecisionType,
		model.CustomerProfileDecisionTargetType,
		profile.ID.UUID(),
		model.UnrestrictedDecisionOutcome,
		[]string{model.CollectionsDueInstallmentsSettledDecisionReason},
		collectionsv1.PolicyID,
		collectionsv1.PolicyVersion,
		profile.Jurisdiction,
		model.SystemDecisionActorType,
		nil,
		&priorDecisionID,
		nil,
		nil,
		now,
		now,
		idempotencyKey,
		fingerprint,
	)
	if err != nil {
		return recoveredEpisode{}, fmt.Errorf("%s: create recovery decision: %w", op, err)
	}

	activeRestriction, err := restrictionQ.LockActiveByCustomerType(
		ctx,
		profile.ID,
		model.NewCreditUsageCustomerRestrictionType,
	)
	if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return recoveredEpisode{}, fmt.Errorf("%s: lock active restriction: %w", op, err)
	}

	if err == nil &&
		activeRestriction.Reason == model.CollectionsDefaultCustomerRestrictionReason &&
		activeRestriction.DefaultEpisodeID != nil &&
		*activeRestriction.DefaultEpisodeID == episode.ID {
		_, releaseErr := restrictionQ.Release(
			ctx,
			activeRestriction.ID,
			decision.ID,
			now,
			now,
		)
		if releaseErr != nil {
			return recoveredEpisode{}, fmt.Errorf("%s: release restriction: %w", op, releaseErr)
		}
	}

	recovered, err := episodeQ.MarkRecovered(ctx, episode.ID, decision.ID, now, now)
	if err != nil {
		return recoveredEpisode{}, fmt.Errorf("%s: mark episode recovered: %w", op, err)
	}

	caseRow, err := caseQ.GetByDefaultEpisodeID(ctx, episode.ID)
	if err != nil {
		return recoveredEpisode{}, fmt.Errorf("%s: load collections case: %w", op, err)
	}

	recoveredPayload, _ := json.Marshal(map[string]any{
		"customer_id":         profile.ID.String(),
		"default_episode_id":  episode.ID.String(),
		"closing_decision_id": decision.ID.String(),
	})
	_, err = caseQ.AppendEvent(ctx, repo.CreateCollectionsCaseEvent{
		ID:         model.CollectionsCaseEventID(uuid.New()),
		CaseID:     caseRow.ID,
		EventType:  model.CustomerRecoveredCollectionsCaseEventType,
		DecisionID: &decision.ID,
		Payload:    recoveredPayload,
		OccurredAt: now,
		ActorType:  model.SystemDecisionActorType,
		CreatedAt:  now,
	})
	if err != nil {
		return recoveredEpisode{}, fmt.Errorf("%s: append recovered event: %w", op, err)
	}

	if err := uc.insertCollectionsAuditOutbox(
		ctx,
		outboxQ,
		"collections.default_episode.recover",
		"default_episode",
		episode.ID.String(),
		decision.ID,
		profile.ID,
		now,
		map[string]any{
			"policy_id":         collectionsv1.PolicyID,
			"policy_version":    collectionsv1.PolicyVersion,
			"reason_code":       model.CollectionsDueInstallmentsSettledDecisionReason,
			"case_id":           caseRow.ID.String(),
			"prior_decision_id": priorDecisionID.String(),
		},
	); err != nil {
		return recoveredEpisode{}, err
	}

	closingID := decision.ID
	if recovered.ClosingDecisionID != nil {
		closingID = *recovered.ClosingDecisionID
	}

	return recoveredEpisode{ClosingDecisionID: closingID}, nil
}

func (uc *UseCase) notePaymentUncertainty(
	ctx context.Context,
	caseQ repo.CollectionsCaseTxQueryer,
	episode model.DefaultEpisode,
	now time.Time,
) error {
	const op = "collectionsuc.UseCase.notePaymentUncertainty"

	caseRow, err := caseQ.GetByDefaultEpisodeID(ctx, episode.ID)
	if err != nil {
		return fmt.Errorf("%s: load collections case: %w", op, err)
	}

	payload, _ := json.Marshal(map[string]any{
		"customer_id":        episode.CustomerID.String(),
		"default_episode_id": episode.ID.String(),
		"noted_at":           now.UTC().Format(time.RFC3339Nano),
	})

	_, err = caseQ.AppendEvent(ctx, repo.CreateCollectionsCaseEvent{
		ID:         model.CollectionsCaseEventID(uuid.New()),
		CaseID:     caseRow.ID,
		EventType:  model.PaymentUncertaintyNotedCollectionsCaseEventType,
		OccurredAt: now,
		ActorType:  model.SystemDecisionActorType,
		Payload:    payload,
		CreatedAt:  now,
	})
	if err != nil {
		return fmt.Errorf("%s: append event: %w", op, err)
	}

	return nil
}

func (uc *UseCase) insertCollectionsAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	action string,
	resourceType string,
	resourceID string,
	decisionID model.DecisionID,
	customerID model.CustomerID,
	now time.Time,
	metadata map[string]any,
) error {
	const op = "collectionsuc.UseCase.insertCollectionsAuditOutbox"

	eventID := uc.idGenerator.Generate()
	correlationID, traceID, err := audit.EnvelopeIDsFromContext(ctx)
	if err != nil {
		// Scheduled assessment has no inbound request envelope; reuse eventID.
		correlationID = eventID
		traceID = eventID
	}

	service := audit.ProducerName
	event := audit.NewEvent(
		eventID,
		correlationID,
		nil,
		traceID,
		now,
		audit.EventData{
			Actor: audit.Actor{
				Type:    audit.SystemActorType,
				Service: &service,
			},
			Action: action,
			Resource: audit.Resource{
				Type: resourceType,
				ID:   resourceID,
			},
			Outcome: audit.SucceededOutcome,
			Metadata: mergeMetadata(metadata, map[string]any{
				"decision_id": decisionID.String(),
				"customer_id": customerID.String(),
			}),
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return fmt.Errorf("%s: validate audit event: %w", op, err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal audit event: %w", op, err)
	}

	aggregateType := resourceType
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    decisionID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert audit outbox: %w", op, err)
	}

	return nil
}

func mergeMetadata(parts ...map[string]any) map[string]any {
	out := make(map[string]any)
	for _, part := range parts {
		for key, value := range part {
			out[key] = value
		}
	}

	return out
}
