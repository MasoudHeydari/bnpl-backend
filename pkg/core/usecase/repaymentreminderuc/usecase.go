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
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	reminderv1 "github.com/parstis/bnpl-backend/pkg/core/policy/repaymentreminder/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
)

// UseCase evaluates installment due schedules and persists at most one reminder
// SMS request per customer per business day via the transactional outbox.
type UseCase struct {
	pool                 repo.Pool
	installment          repo.Installment
	repaymentReminder    repo.RepaymentReminder
	paymentAttempt       repo.PaymentAttempt
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
}

// New constructs a repayment reminder use case.
func New(
	pool repo.Pool,
	installment repo.Installment,
	repaymentReminder repo.RepaymentReminder,
	paymentAttempt repo.PaymentAttempt,
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

	if repaymentReminder == nil {
		return nil, fmt.Errorf("%s: repayment reminder repository is required", op)
	}

	if paymentAttempt == nil {
		return nil, fmt.Errorf("%s: payment attempt repository is required", op)
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

	return &UseCase{
		pool:                 pool,
		installment:          installment,
		repaymentReminder:    repaymentReminder,
		paymentAttempt:       paymentAttempt,
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

	// Cover D-1 through D+MaxGrace: due dates from tomorrow back through
	// MaxGracePeriodDays before today, expressed as UTC instants spanning those
	// calendar days in the business location.
	dueFromLocal := businessDate.AddDate(0, 0, -int(model.MaxInstallmentGracePeriodDays))
	dueToLocal := businessDate.AddDate(0, 0, 2)
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

		// Deterministic customer order for stable progress and testability.
		customerIDs := make([]model.CustomerID, 0, pageCustomerCount)
		for customerID := range grouped {
			customerIDs = append(customerIDs, customerID)
		}

		sortCustomerIDs(customerIDs)

		for _, customerID := range customerIDs {
			group := grouped[customerID]
			created, skipReason, dispatchErr := uc.dispatchCustomer(
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

			if created {
				output.CreatedCount++
				continue
			}

			output.SkippedCount++
			if skipReason != "" {
				uc.businessMetrics.RepaymentReminderSkipped(ctx, string(skipReason))
			}
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

// errConcurrentCustomerDayReminder forces a transaction rollback when another
// worker already persisted the customer/day Reminder, so the Short Link insert
// from the losing attempt is not committed as an orphan.
var errConcurrentCustomerDayReminder = errors.New("concurrent customer-day reminder")

func (uc *UseCase) dispatchCustomer(
	ctx context.Context,
	nowLocal time.Time,
	businessDate time.Time,
	customerID model.CustomerID,
	group customerCandidateGroup,
) (bool, reminderv1.SuppressReason, error) {
	const op = "repaymentreminderuc.UseCase.dispatchCustomer"

	var (
		created      bool
		skipReason   reminderv1.SuppressReason
		reminderType model.ReminderType
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			reminderQ := uc.repaymentReminder.Tx(tx)
			installmentQ := uc.installment.Tx(tx)
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)

			_, lockErr := reminderQ.LockByCustomerBusinessDate(ctx, customerID, businessDate)
			if lockErr == nil {
				skipReason = reminderv1.SuppressAlreadySentToday
				return nil
			}

			if !cerr.IsCode(lockErr, cerr.CodeResourceNotFound) {
				return lockErr
			}

			// Revalidate eligibility inside the transaction before committing.
			page := pagination.KeysetRequest{Limit: 500, Order: pagination.OrderAsc}
			page.Normalize()
			rows, listErr := installmentQ.ListUnpaidByCustomerID(ctx, customerID, page)
			if listErr != nil {
				return listErr
			}

			installmentIDs := make([]model.InstallmentID, 0, len(rows))
			for _, row := range rows {
				installmentIDs = append(installmentIDs, row.ID)
			}

			pendingIDs, pendingErr := paymentAttemptQ.ListInstallmentIDsWithNonTerminalAttempts(
				ctx,
				customerID,
				installmentIDs,
			)
			if pendingErr != nil {
				return pendingErr
			}

			pendingByID := make(map[model.InstallmentID]bool, len(pendingIDs))
			for _, id := range pendingIDs {
				pendingByID[id] = true
			}

			facts := make([]reminderv1.InstallmentFact, 0, len(rows))
			for _, row := range rows {
				facts = append(facts, reminderv1.InstallmentFact{
					InstallmentID:        row.ID,
					DueAt:                row.DueAt,
					GracePeriodDays:      row.GracePeriodDays,
					Status:               row.Status,
					HasUnresolvedPayment: pendingByID[row.ID],
				})
			}

			decision := uc.policy.EvaluateCustomer(nowLocal, businessDate, facts, false)
			if !decision.ShouldSend {
				skipReason = decision.SuppressReason
				return nil
			}

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

			idempotencyKey := "repayment-reminder:" + reminderID.String()
			nowUTC := uc.clock.Now().UTC()

			_, createErr := reminderQ.Create(ctx, repo.CreateRepaymentReminder{
				ID:                         reminderID,
				CustomerID:                 customerID,
				ExternalCustomerID:         group.ExternalCustomerID,
				BusinessDate:               businessDate,
				ReminderType:               decision.ReminderType,
				PolicyVersion:              decision.PolicyVersion,
				TemplateID:                 decision.TemplateID,
				ShortLinkID:                linkOut.ShortLink.ID,
				PaymentURL:                 paymentURL,
				InstallmentIDs:             decision.InstallmentIDs,
				NotificationIdempotencyKey: idempotencyKey,
				CreatedAt:                  nowUTC,
			})
			if createErr != nil {
				if cerr.IsCode(createErr, cerr.CodeConflict) {
					// Concurrent worker won the customer/day race. Return an
					// error so the transaction rolls back the Short Link insert.
					return errConcurrentCustomerDayReminder
				}

				return createErr
			}

			eventID := uc.idGenerator.Generate()
			event := notificationsms.NewEvent(
				eventID,
				idempotencyKey,
				nowUTC,
				group.ExternalCustomerID.String(),
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
				MessageKey:    group.ExternalCustomerID.String(),
				EventType:     notificationsms.EventType,
				AggregateType: &aggregateType,
				Payload:       payload,
				CreatedAt:     nowUTC,
			})
			if outboxErr != nil {
				return outboxErr
			}

			created = true
			reminderType = decision.ReminderType
			return nil
		})
	})
	if err != nil {
		if errors.Is(err, errConcurrentCustomerDayReminder) {
			return false, reminderv1.SuppressAlreadySentToday, nil
		}

		return false, "", err
	}

	if created {
		uc.businessMetrics.ShortLinkCreated(ctx, "generated")
		uc.businessMetrics.RepaymentReminderCreated(ctx, string(reminderType))
	}

	return created, skipReason, nil
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
