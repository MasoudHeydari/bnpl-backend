package repaymentreminderuc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/event/notificationsms"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/metrics"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	notificationv1 "github.com/parstis/bnpl-backend/pkg/core/policy/defaultnotification/v1"
	reminderv1 "github.com/parstis/bnpl-backend/pkg/core/policy/repaymentreminder/v1"
	"github.com/parstis/bnpl-backend/pkg/core/policy/shortlinkdestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
)

// UseCase evaluates installment due schedules and persists every independently
// eligible reminder SMS request via the transactional outbox. Aggregation and
// customer-day SMS ceilings are forbidden; semantic idempotency keys dedupe.
type DefaultNoticeEvaluator interface {
	DefaultNoticeOnTx(context.Context, repo.Tx, model.CustomerID, time.Time) (notificationv1.Eligibility, error)
}

type UseCase struct {
	defaultNotice        DefaultNoticeEvaluator
	pool                 repo.Pool
	installment          repo.Installment
	contract             repo.Contract
	repaymentReminder    repo.RepaymentReminder
	paymentAttempt       repo.PaymentAttempt
	decision             repo.Decision
	checkoutPolicies     *checkout.Registry
	outbox               repo.Outbox
	shortLink            *shortlinkuc.UseCase
	clock                clock.Clock
	idGenerator          identifier.Generator
	businessMetrics      metrics.BusinessMetrics
	policy               reminderv1.Policy
	notificationTopic    string
	publicHost           string
	repaymentDestination string
	batchSize            int
	maxReminderLeadDays  uint
}

// New constructs a repayment reminder use case.
func New(
	pool repo.Pool,
	installment repo.Installment,
	contract repo.Contract,
	repaymentReminder repo.RepaymentReminder,
	paymentAttempt repo.PaymentAttempt,
	decision repo.Decision,
	checkoutPolicies *checkout.Registry,
	outbox repo.Outbox,
	shortLink *shortlinkuc.UseCase,
	clk clock.Clock,
	idGenerator identifier.Generator,
	businessMetrics metrics.BusinessMetrics,
	notificationTopic string,
	publicHost string,
	repaymentDestination string,
	batchSize int,
) (*UseCase, error) {
	const op = "repaymentreminderuc.New"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if installment == nil {
		return nil, fmt.Errorf("%s: installment repository is required", op)
	}

	if contract == nil {
		return nil, fmt.Errorf("%s: contract repository is required", op)
	}

	if repaymentReminder == nil {
		return nil, fmt.Errorf("%s: repayment reminder repository is required", op)
	}

	if paymentAttempt == nil {
		return nil, fmt.Errorf("%s: payment attempt repository is required", op)
	}

	if decision == nil {
		return nil, fmt.Errorf("%s: decision repository is required", op)
	}

	if checkoutPolicies == nil {
		return nil, fmt.Errorf("%s: checkout policy registry is required", op)
	}

	if outbox == nil {
		return nil, fmt.Errorf("%s: outbox repository is required", op)
	}

	if shortLink == nil {
		return nil, fmt.Errorf("%s: short link use case is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: id generator is required", op)
	}

	if businessMetrics == nil {
		return nil, fmt.Errorf("%s: business metrics is required", op)
	}

	if strings.TrimSpace(notificationTopic) == "" {
		return nil, fmt.Errorf("%s: notification topic is required", op)
	}

	// Public host and repayment destination may be empty when reminder dispatch
	// is disabled. DispatchBatch fail-closes if they are missing so SMS cannot
	// be sent with an unusable short link.
	if batchSize <= 0 {
		return nil, fmt.Errorf("%s: batch size must be positive", op)
	}

	if err := reminderv1.ValidateTerms(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	maxLead, err := checkoutPolicies.MaxReminderLeadDays()
	if err != nil {
		return nil, fmt.Errorf("%s: checkout reminder discovery horizon: %w", op, err)
	}

	return &UseCase{
		pool:                 pool,
		installment:          installment,
		contract:             contract,
		repaymentReminder:    repaymentReminder,
		paymentAttempt:       paymentAttempt,
		decision:             decision,
		checkoutPolicies:     checkoutPolicies,
		outbox:               outbox,
		shortLink:            shortLink,
		clock:                clk,
		idGenerator:          idGenerator,
		businessMetrics:      businessMetrics,
		policy:               reminderv1.New(),
		notificationTopic:    strings.TrimSpace(notificationTopic),
		publicHost:           strings.TrimSpace(strings.TrimSuffix(publicHost, "/")),
		repaymentDestination: strings.TrimSpace(repaymentDestination),
		batchSize:            batchSize,
		maxReminderLeadDays:  maxLead,
	}, nil
}

// DispatchOutput reports one reminder dispatch run.
type DispatchOutput struct {
	// CandidateCount is the number of installment candidates loaded.
	CandidateCount int

	// EligibleCustomerCount is the number of distinct customers evaluated.
	EligibleCustomerCount int

	// CreatedCount is the number of reminder requests persisted.
	CreatedCount int

	// SkippedCount is the number of customers skipped by policy or dedupe.
	SkippedCount int
}

// Dispatch evaluates due schedules and persists reminder outbox events.
//
// Customers are processed in deterministic keyset pages so every eligible
// customer-day is eventually examined even when unpaid installments remain in
// earlier pages across repeated scheduler executions.
func (uc *UseCase) Dispatch(ctx context.Context) (DispatchOutput, error) {
	const op = "repaymentreminderuc.UseCase.Dispatch"

	if strings.TrimSpace(uc.publicHost) == "" {
		return DispatchOutput{}, fmt.Errorf("%s: public host is required for reminder short links", op)
	}

	if err := shortlinkdestination.ValidatePublicHost(uc.publicHost); err != nil {
		return DispatchOutput{}, fmt.Errorf("%s: %w", op, err)
	}

	if strings.TrimSpace(uc.repaymentDestination) == "" {
		return DispatchOutput{}, fmt.Errorf("%s: repayment destination url is required for reminder short links", op)
	}

	started := uc.clock.Now().UTC()
	loc := uc.clock.BusinessLocation()
	nowLocal := started.In(loc)
	businessDate := time.Date(
		nowLocal.Year(),
		nowLocal.Month(),
		nowLocal.Day(),
		0, 0, 0, 0,
		loc,
	)

	// Candidate discovery is intentionally broader than today's exact reminder
	// eligibility. An installment can require a notification before its due
	// date (Pre-Due lead), on its due date, or during its contractual grace
	// period / active default episode.
	//
	// Past bound: look back through the operational MaxInstallmentGracePeriodDays
	// floor so historical grace snapshots and active default episodes remain
	// discoverable. The SQL predicate also keeps longer persisted grace windows
	// and active default episodes even when due_at falls before that floor.
	//
	// Future bound: include installments due up to the largest registered
	// Checkout Policy Reminder.LeadDays. Repository queries use a half-open
	// interval [dueAtFrom, dueAtTo), so LeadDays=3 requires an exclusive upper
	// bound at the start of local day D+(LeadDays+1).
	//
	// The discovery envelope is not authorization to send. Each candidate is
	// evaluated against its originating Checkout Policy terms during the
	// customer-scoped transaction using current payment and default facts.
	dueFromLocal := businessDate.AddDate(0, 0, -int(model.MaxInstallmentGracePeriodDays))
	dueToLocal := businessDate.AddDate(0, 0, int(uc.maxReminderLeadDays)+1)
	dueAtFrom := dueFromLocal.UTC()
	dueAtTo := dueToLocal.UTC()

	var output DispatchOutput
	afterCustomerID := model.CustomerID{}

	for {
		var candidates []repo.ReminderCandidate

		err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			installmentQ := uc.installment.Conn(conn)
			rows, listErr := installmentQ.ListReminderCandidatePage(
				ctx,
				dueAtFrom,
				dueAtTo,
				businessDate,
				afterCustomerID,
				uc.batchSize,
				loc.String(),
			)
			if listErr != nil {
				return listErr
			}

			candidates = rows
			return nil
		})
		if err != nil {
			uc.businessMetrics.RepaymentReminderJobCompleted(ctx, "failure")
			uc.businessMetrics.RepaymentReminderJobDuration(ctx, uc.clock.Now().UTC().Sub(started))
			return output, fmt.Errorf("%s: list candidates: %w", op, err)
		}

		if len(candidates) == 0 {
			break
		}

		output.CandidateCount += len(candidates)
		uc.businessMetrics.RepaymentReminderCandidatesObserved(ctx, len(candidates))

		grouped := groupCandidatesByCustomer(candidates)
		pageCustomerCount := len(grouped)
		output.EligibleCustomerCount += pageCustomerCount
		uc.businessMetrics.RepaymentReminderEligibleCustomersObserved(ctx, pageCustomerCount)

		// Page by customer so every candidate installment already returned by
		// ListReminderCandidatePage is evaluated together. Multiple independent
		// notifications may be created for one customer/day; priority only
		// orders processing. Final send authority remains persistNotification.
		customerIDs := make([]model.CustomerID, 0, pageCustomerCount)
		for customerID := range grouped {
			customerIDs = append(customerIDs, customerID)
		}

		sortCustomerIDs(customerIDs)

		for _, customerID := range customerIDs {
			group := grouped[customerID]
			createdCount, skippedCount, dispatchErr := uc.dispatchCustomer(
				ctx,
				nowLocal,
				businessDate,
				customerID,
				group,
			)
			if dispatchErr != nil {
				uc.businessMetrics.RepaymentReminderJobCompleted(ctx, "failure")
				uc.businessMetrics.RepaymentReminderJobDuration(ctx, uc.clock.Now().UTC().Sub(started))
				return output, fmt.Errorf("%s: dispatch customer: %w", op, dispatchErr)
			}

			output.CreatedCount += createdCount
			output.SkippedCount += skippedCount
		}

		afterCustomerID = customerIDs[len(customerIDs)-1]
		if pageCustomerCount < uc.batchSize {
			break
		}
	}

	uc.businessMetrics.RepaymentReminderJobCompleted(ctx, "success")
	uc.businessMetrics.RepaymentReminderJobDuration(ctx, uc.clock.Now().UTC().Sub(started))

	return output, nil
}

type customerCandidateGroup struct {
	ExternalCustomerID model.ExternalCustomerID
	Candidates         []repo.ReminderCandidate
}

func groupCandidatesByCustomer(
	candidates []repo.ReminderCandidate,
) map[model.CustomerID]customerCandidateGroup {
	grouped := make(map[model.CustomerID]customerCandidateGroup, len(candidates))
	for _, candidate := range candidates {
		customerID := candidate.Installment.CustomerID
		group := grouped[customerID]
		group.ExternalCustomerID = candidate.ExternalCustomerID
		group.Candidates = append(group.Candidates, candidate)
		grouped[customerID] = group
	}

	return grouped
}

func sortCustomerIDs(ids []model.CustomerID) {
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
}

// errConcurrentSemanticReminder forces a transaction rollback when another
// worker already persisted the same semantic notification identity, so the
// Short Link insert from the losing attempt is not committed as an orphan.
var errConcurrentSemanticReminder = errors.New("concurrent semantic reminder")

func (uc *UseCase) dispatchCustomer(
	ctx context.Context,
	nowLocal time.Time,
	businessDate time.Time,
	customerID model.CustomerID,
	group customerCandidateGroup,
) (createdCount int, skippedCount int, err error) {
	const op = "repaymentreminderuc.UseCase.dispatchCustomer"

	decisions, loadErr := uc.loadEligibleNotifications(
		ctx,
		nowLocal,
		businessDate,
		customerID,
		group.Candidates,
	)
	if loadErr != nil {
		return 0, 0, loadErr
	}

	if len(decisions) == 0 {
		return 0, 1, nil
	}

	for _, decision := range decisions {
		created, persistErr := uc.persistNotification(ctx, customerID, group.ExternalCustomerID, decision)
		if persistErr != nil {
			// A failure on one notification must not permanently drop later
			// independently eligible events in the same customer pass.
			return createdCount, skippedCount, fmt.Errorf(
				"%s: persist %s for installment %s: %w",
				op,
				decision.ReminderType,
				decision.InstallmentID,
				persistErr,
			)
		}

		if created {
			createdCount++
			uc.businessMetrics.ShortLinkCreated(ctx, "generated")
			uc.businessMetrics.RepaymentReminderCreated(ctx, string(decision.ReminderType))
			continue
		}

		skippedCount++
		uc.businessMetrics.RepaymentReminderSkipped(ctx, string(reminderv1.SuppressAlreadySent))
	}

	return createdCount, skippedCount, nil
}

// loadEligibleNotifications turns discovery candidates into independent
// NotificationDecisions. Candidates already carry installment rows and
// HasUnresolvedPayment from ListReminderCandidatePage; they are not re-queried
// here. That discovery data is advisory only — persistNotification revalidates
// under locks before ShortLink/Reminder/Outbox commit.
//
// Default eligibility still runs through DefaultNoticeOnTx. An active Default
// Episode may authorize installment IDs that are not ordinary due-date
// reminders, so Default is never derived only from group.Candidates.
func (uc *UseCase) loadEligibleNotifications(
	ctx context.Context,
	nowLocal time.Time,
	businessDate time.Time,
	customerID model.CustomerID,
	candidates []repo.ReminderCandidate,
) ([]reminderv1.NotificationDecision, error) {
	const op = "repaymentreminderuc.UseCase.loadEligibleNotifications"

	var decisions []reminderv1.NotificationDecision
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			decisionQ := uc.decision.Tx(tx)

			rows := make([]model.Installment, 0, len(candidates))
			for _, candidate := range candidates {
				if candidate.Installment.CustomerID != customerID {
					continue
				}

				rows = append(rows, candidate.Installment)
			}

			leadByDecision, leadErr := uc.reminderLeadDaysByDecision(ctx, decisionQ, rows)
			if leadErr != nil {
				return leadErr
			}

			facts := make([]reminderv1.InstallmentFact, 0, len(candidates))
			for _, candidate := range candidates {
				row := candidate.Installment
				if row.CustomerID != customerID {
					continue
				}

				facts = append(facts, reminderv1.InstallmentFact{
					InstallmentID:        row.ID,
					DueAt:                row.DueAt,
					GracePeriodDays:      row.GracePeriodDays,
					LeadDays:             leadByDecision[row.DecisionID],
					Status:               row.Status,
					HasUnresolvedPayment: candidate.HasUnresolvedPayment,
				})
			}

			decisions = uc.policy.EvaluateAll(nowLocal, businessDate, facts)

			if uc.defaultNotice == nil {
				return nil
			}

			// Reserved remains informational for observability inside
			// DefaultNoticeOnTx; it must not suppress independently eligible
			// ordinary reminders. Each Default-triggering installment gets its
			// own semantic identity under the episode.
			notice, noticeErr := uc.defaultNotice.DefaultNoticeOnTx(ctx, tx, customerID, nowLocal)
			if noticeErr != nil {
				return noticeErr
			}

			if !notice.Eligible {
				return nil
			}

			episodeID := notice.EpisodeID
			for _, installmentID := range notice.InstallmentIDs {
				if installmentID.IsZero() {
					continue
				}

				decisions = append(decisions, reminderv1.NotificationDecision{
					InstallmentID: installmentID,
					ReminderType:  model.DefaultReminderType,
					TemplateID:    notificationv1.TemplateID,
					BusinessDate:  businessDate,
					PolicyVersion: notificationv1.PolicyVersion,
					EpisodeID:     &episodeID,
				})
			}

			sort.SliceStable(decisions, func(i, j int) bool {
				pi := decisions[i].ReminderType.Priority()
				pj := decisions[j].ReminderType.Priority()
				if pi != pj {
					return pi > pj
				}

				return decisions[i].InstallmentID.String() < decisions[j].InstallmentID.String()
			})

			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return decisions, nil
}

// notificationStillEligible rechecks one candidate under the persistence
// transaction's locks. A negative result skips the insert; it does not
// suppress any other installment's independent notification.
func (uc *UseCase) notificationStillEligible(
	ctx context.Context,
	tx repo.Tx,
	customerID model.CustomerID,
	decision reminderv1.NotificationDecision,
) (bool, error) {
	const op = "repaymentreminderuc.UseCase.notificationStillEligible"

	// Lock order:
	//   Default path: Customer Profile (via DefaultNoticeOnTx) → Installment
	//   Ordinary path: Contract → Installment
	//
	// Contract → Installment matches cancellation (after customer) and
	// installment payment settlement. Never lock Installment then Contract:
	// cancel/settlement would deadlock. Ordinary reminders do not take the
	// customer lock; DefaultNoticeOnTx does. Taking Installment then Customer
	// deadlocks with collections assessment.
	loc := uc.clock.BusinessLocation()
	nowLocal := uc.clock.Now().In(loc)
	businessDate := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)

	if decision.ReminderType == model.DefaultReminderType {
		if uc.defaultNotice == nil || decision.EpisodeID == nil {
			return false, nil
		}

		notice, noticeErr := uc.defaultNotice.DefaultNoticeOnTx(ctx, tx, customerID, nowLocal)
		if noticeErr != nil {
			return false, noticeErr
		}

		if !notice.Eligible || notice.EpisodeID != *decision.EpisodeID {
			return false, nil
		}

		installmentQ := uc.installment.Tx(tx)
		locked, err := installmentQ.LockByID(ctx, decision.InstallmentID)
		if err != nil {
			return false, fmt.Errorf("%s: lock installment: %w", op, err)
		}

		if locked.CustomerID != customerID || locked.Status.IsTerminal() {
			return false, nil
		}

		for _, id := range notice.InstallmentIDs {
			if id == locked.ID {
				return true, nil
			}
		}

		return false, nil
	}

	// Ordinary Pre-Due / DueToday / Grace* require a currently active Contract.
	// Discovery may still surface canceling-contract installments when an
	// active Default Episode exists; that path must not authorize ordinary SMS.
	//
	// READ COMMITTED: an unlocked installment read only discovers ContractID.
	// Status authority comes from locking Contract before Installment so a
	// concurrent cancel that already holds the contract row either blocks us
	// until canceling is visible, or waits behind our lock until we suppress
	// or commit against a still-active contract.
	installmentQ := uc.installment.Tx(tx)
	unlocked, err := installmentQ.GetByID(ctx, decision.InstallmentID)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("%s: load installment: %w", op, err)
	}

	if unlocked.CustomerID != customerID || unlocked.Status.IsTerminal() {
		return false, nil
	}

	if unlocked.ContractID == nil || unlocked.ContractID.IsZero() {
		return false, nil
	}

	contractID := *unlocked.ContractID
	contractQ := uc.contract.Tx(tx)
	contractRow, err := contractQ.LockByID(ctx, contractID)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("%s: lock contract: %w", op, err)
	}

	if contractRow.CustomerID != customerID {
		return false, nil
	}

	if !contractRow.Status.IsActive() {
		return false, nil
	}

	locked, err := installmentQ.LockByID(ctx, decision.InstallmentID)
	if err != nil {
		return false, fmt.Errorf("%s: lock installment: %w", op, err)
	}

	if locked.CustomerID != customerID || locked.Status.IsTerminal() {
		return false, nil
	}

	if locked.ContractID == nil || *locked.ContractID != contractID {
		return false, nil
	}

	paymentAttemptQ := uc.paymentAttempt.Tx(tx)
	pendingIDs, err := paymentAttemptQ.ListInstallmentIDsWithNonTerminalAttempts(
		ctx,
		customerID,
		[]model.InstallmentID{locked.ID},
	)
	if err != nil {
		return false, fmt.Errorf("%s: list unresolved payments: %w", op, err)
	}

	pending := len(pendingIDs) > 0

	if pending {
		return false, nil
	}

	decisionQ := uc.decision.Tx(tx)
	leadByDecision, leadErr := uc.reminderLeadDaysByDecision(ctx, decisionQ, []model.Installment{locked})
	if leadErr != nil {
		return false, leadErr
	}

	facts := []reminderv1.InstallmentFact{{
		InstallmentID:        locked.ID,
		DueAt:                locked.DueAt,
		GracePeriodDays:      locked.GracePeriodDays,
		LeadDays:             leadByDecision[locked.DecisionID],
		Status:               locked.Status,
		HasUnresolvedPayment: pending,
	}}
	for _, next := range uc.policy.EvaluateAll(nowLocal, businessDate, facts) {
		if next.InstallmentID == decision.InstallmentID &&
			next.ReminderType == decision.ReminderType &&
			next.BusinessDate.Equal(decision.BusinessDate) {
			return true, nil
		}
	}

	return false, nil
}

func (uc *UseCase) persistNotification(
	ctx context.Context,
	customerID model.CustomerID,
	externalCustomerID model.ExternalCustomerID,
	decision reminderv1.NotificationDecision,
) (bool, error) {
	const op = "repaymentreminderuc.UseCase.persistNotification"

	idempotencyKey := reminderv1.NotificationIdempotencyKey(
		decision.InstallmentID,
		decision.ReminderType,
		decision.BusinessDate,
	)
	if decision.ReminderType == model.DefaultReminderType {
		if decision.EpisodeID == nil || decision.EpisodeID.IsZero() {
			return false, fmt.Errorf("%s: default notification requires episode id", op)
		}

		idempotencyKey = reminderv1.DefaultNotificationIdempotencyKey(
			*decision.EpisodeID,
			decision.InstallmentID,
		)
	}

	created := false
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			reminderQ := uc.repaymentReminder.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)

			_, getErr := reminderQ.GetByNotificationIdempotencyKey(ctx, idempotencyKey)
			if getErr == nil {
				return nil
			}

			if !cerr.IsCode(getErr, cerr.CodeResourceNotFound) {
				return getErr
			}

			// Discovery ran in an earlier transaction. Re-read the locked
			// installment and current payment/default facts before inserting
			// anything, so a payment that committed in between cannot still
			// produce a new SMS request.
			eligible, eligibleErr := uc.notificationStillEligible(ctx, tx, customerID, decision)
			if eligibleErr != nil {
				return eligibleErr
			}

			if !eligible {
				return nil
			}

			// ShortLink, Reminder row, and Outbox event must commit together so
			// a partial failure cannot leave an orphan redirect target or an
			// SMS request without durable delivery evidence (and vice versa).
			linkOut, linkErr := uc.shortLink.CreateOnTx(ctx, tx, shortlinkuc.CreateInput{
				DestinationURL: uc.repaymentDestination,
			})
			if linkErr != nil {
				return fmt.Errorf("%s: create short link: %w", op, linkErr)
			}

			paymentURL := displayShortURL(uc.publicHost, linkOut.ShortLink.Code)
			reminderID, idErr := parseReminderID(uc.idGenerator.Generate())
			if idErr != nil {
				return fmt.Errorf("%s: parse reminder id: %w", op, idErr)
			}

			nowUTC := uc.clock.Now().UTC()
			_, createErr := reminderQ.Create(ctx, repo.CreateRepaymentReminder{
				ID:                         reminderID,
				CustomerID:                 customerID,
				ExternalCustomerID:         externalCustomerID,
				BusinessDate:               decision.BusinessDate,
				ReminderType:               decision.ReminderType,
				PolicyVersion:              decision.PolicyVersion,
				TemplateID:                 decision.TemplateID,
				ShortLinkID:                linkOut.ShortLink.ID,
				PaymentURL:                 paymentURL,
				InstallmentIDs:             []model.InstallmentID{decision.InstallmentID},
				NotificationIdempotencyKey: idempotencyKey,
				CreatedAt:                  nowUTC,
			})
			if createErr != nil {
				if cerr.IsCode(createErr, cerr.CodeConflict) {
					return errConcurrentSemanticReminder
				}

				return createErr
			}

			eventID := uc.idGenerator.Generate()
			event := notificationsms.NewEvent(
				eventID,
				idempotencyKey,
				nowUTC,
				externalCustomerID.String(),
				decision.TemplateID,
				map[string]string{
					"payment_url": paymentURL,
				},
			)
			if validateErr := notificationsms.ValidateEvent(event); validateErr != nil {
				return fmt.Errorf("%s: validate notification event: %w", op, validateErr)
			}

			payload, marshalErr := json.Marshal(event)
			if marshalErr != nil {
				return fmt.Errorf("%s: marshal notification event: %w", op, marshalErr)
			}

			aggregateType := "repayment_reminder"
			_, outboxErr := outboxQ.Create(ctx, repo.CreateOutboxMessage{
				Destination:   uc.notificationTopic,
				MessageKey:    externalCustomerID.String(),
				EventType:     notificationsms.EventType,
				AggregateType: &aggregateType,
				Payload:       payload,
				CreatedAt:     nowUTC,
			})
			if outboxErr != nil {
				return outboxErr
			}

			created = true
			return nil
		})
	})
	if err != nil {
		if errors.Is(err, errConcurrentSemanticReminder) {
			return false, nil
		}

		return false, err
	}

	return created, nil
}

func (uc *UseCase) reminderLeadDaysByDecision(
	ctx context.Context,
	decisionQ repo.DecisionTxQueryer,
	rows []model.Installment,
) (map[model.DecisionID]uint, error) {
	const op = "repaymentreminderuc.UseCase.reminderLeadDaysByDecision"

	out := make(map[model.DecisionID]uint, len(rows))
	for _, row := range rows {
		if _, exists := out[row.DecisionID]; exists {
			continue
		}

		decision, err := decisionQ.GetByID(ctx, row.DecisionID)
		if err != nil {
			return nil, fmt.Errorf("%s: load decision %s: %w", op, row.DecisionID, err)
		}

		selected, err := uc.checkoutPolicies.Resolve(policy.Ref{
			ID:      decision.PolicyID,
			Version: decision.PolicyVersion,
		})
		if err != nil {
			return nil, fmt.Errorf("%s: resolve checkout policy for decision %s: %w", op, row.DecisionID, err)
		}

		lead := selected.Terms().Repayment.Reminder.LeadDays
		if lead == 0 {
			return nil, fmt.Errorf("%s: checkout policy %s/%s has zero reminder lead days", op, decision.PolicyID, decision.PolicyVersion)
		}

		out[row.DecisionID] = lead
	}

	return out, nil
}

func displayShortURL(publicHost, code string) string {
	host := strings.TrimSpace(strings.TrimSuffix(publicHost, "/"))
	code = strings.TrimSpace(strings.TrimPrefix(code, "/"))
	return host + "/" + code
}

func parseReminderID(raw string) (model.RepaymentReminderID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return model.RepaymentReminderID{}, err
	}

	return model.RepaymentReminderID(id), nil
}

// WithDefaultNotifications wires episode-scoped Default notices as independent
// notifications alongside ordinary reminders. Default eligibility never imposes
// a customer-day SMS ceiling or suppresses other eligible installment messages.
func (uc *UseCase) WithDefaultNotifications(e DefaultNoticeEvaluator) *UseCase {
	uc.defaultNotice = e
	return uc
}
