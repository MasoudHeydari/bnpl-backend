package v1

import (
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	// PolicyVersion identifies this repayment reminder policy revision.
	PolicyVersion = "repayment_reminder.v1"

	// MaxMessagesPerCustomerPerDay is the durable daily SMS ceiling.
	MaxMessagesPerCustomerPerDay = 1

	// PreDueHour is the local hour for D-1 reminders.
	PreDueHour = 10

	// DueTodayHour is the local hour for D0 reminders.
	DueTodayHour = 10

	// GraceHour is the local hour for grace-period reminders (D+1..D+Grace).
	GraceHour = 13

	// TemplatePreDue is the Notification Service template for D-1 reminders.
	TemplatePreDue = "bnpl.repayment.pre_due.v1"

	// TemplateDueToday is the Notification Service template for D0 reminders.
	TemplateDueToday = "bnpl.repayment.due_today.v1"

	// TemplateGracePeriod is the Notification Service template for mid-grace reminders.
	TemplateGracePeriod = "bnpl.repayment.grace_period.v1"

	// TemplateGraceFinalDay is the Notification Service template for final grace day.
	TemplateGraceFinalDay = "bnpl.repayment.grace_final_day.v1"
)

// SuppressReason explains why no reminder should be created.
type SuppressReason string

// Known suppression reasons (bounded, safe for metrics labels).
const (
	SuppressNone             SuppressReason = ""
	SuppressAlreadySentToday SuppressReason = "already_sent_today"
	SuppressNoEligible       SuppressReason = "no_eligible"
	SuppressBeforeWindow     SuppressReason = "before_window"
	SuppressStaleWindow      SuppressReason = "stale_window"
	SuppressPendingPayment   SuppressReason = "pending_payment"
	SuppressNotUnpaid        SuppressReason = "not_unpaid"
)

// InstallmentFact carries the explicit facts needed to evaluate one installment.
type InstallmentFact struct {
	// InstallmentID identifies the installment.
	InstallmentID model.InstallmentID

	// DueAt is the contractual due instant (interpreted in business location).
	DueAt time.Time

	// GracePeriodDays is the contractual grace snapshot from the installment.
	GracePeriodDays uint

	// Status is the current installment lifecycle status.
	Status model.InstallmentStatus

	// HasUnresolvedPayment is true when a non-terminal payment attempt exists.
	HasUnresolvedPayment bool
}

// CustomerDecision is the pure policy outcome for one customer on one business day.
type CustomerDecision struct {
	// ShouldSend reports whether a reminder request should be persisted.
	ShouldSend bool

	// ReminderType is the selected semantic type when ShouldSend is true.
	ReminderType model.ReminderType

	// TemplateID is the Notification Service template when ShouldSend is true.
	TemplateID string

	// InstallmentIDs lists installments that justified the selected type.
	// Only installments matching the winning type are included so a final-day
	// warning never misdescribes unrelated earlier-stage installments.
	InstallmentIDs []model.InstallmentID

	// SuppressReason explains why sending was suppressed when ShouldSend is false.
	SuppressReason SuppressReason

	// PolicyVersion identifies this policy revision.
	PolicyVersion string
}

// Policy is the v1 repayment reminder policy.
type Policy struct{}

// New returns the v1 repayment reminder policy.
func New() Policy {
	return Policy{}
}

// EvaluateCustomer selects at most one reminder for a customer on the given
// business calendar day.
//
// now and businessDate must already be expressed with the business timezone
// location (Asia/Tehran by default). businessDate should be truncated to the
// calendar date in that location.
func (Policy) EvaluateCustomer(
	now time.Time,
	businessDate time.Time,
	facts []InstallmentFact,
	alreadySentToday bool,
) CustomerDecision {
	decision := CustomerDecision{
		PolicyVersion: PolicyVersion,
	}

	if alreadySentToday {
		decision.SuppressReason = SuppressAlreadySentToday
		return decision
	}

	loc := now.Location()
	if loc == nil {
		loc = time.UTC
	}

	businessDay := truncateDate(businessDate.In(loc))
	nowLocal := now.In(loc)

	var (
		bestType   model.ReminderType
		bestIDs    []model.InstallmentID
		sawPending bool
		sawAny     bool
		beforeOnly bool
		staleOnly  bool
	)

	for _, fact := range facts {
		slot, reason := evaluateInstallment(nowLocal, businessDay, fact)
		switch reason {
		case SuppressPendingPayment:
			sawPending = true
			continue

		case SuppressNotUnpaid:
			continue

		case SuppressBeforeWindow:
			beforeOnly = true
			continue

		case SuppressStaleWindow:
			staleOnly = true
			continue

		case SuppressNoEligible:
			continue
		}

		if slot == "" {
			continue
		}

		sawAny = true
		if bestType == "" || slot.Priority() > bestType.Priority() {
			bestType = slot
			bestIDs = []model.InstallmentID{fact.InstallmentID}
			continue
		}

		if slot == bestType {
			bestIDs = append(bestIDs, fact.InstallmentID)
		}
	}

	if bestType == "" {
		switch {
		case sawPending && !sawAny:
			decision.SuppressReason = SuppressPendingPayment

		case beforeOnly && !staleOnly:
			decision.SuppressReason = SuppressBeforeWindow

		case staleOnly:
			decision.SuppressReason = SuppressStaleWindow

		default:
			decision.SuppressReason = SuppressNoEligible
		}

		return decision
	}

	decision.ShouldSend = true
	decision.ReminderType = bestType
	decision.TemplateID = templateFor(bestType)
	decision.InstallmentIDs = bestIDs

	return decision
}

func evaluateInstallment(
	nowLocal time.Time,
	businessDay time.Time,
	fact InstallmentFact,
) (model.ReminderType, SuppressReason) {
	if !fact.Status.IsActive() {
		return "", SuppressNotUnpaid
	}

	if fact.HasUnresolvedPayment {
		return "", SuppressPendingPayment
	}

	loc := nowLocal.Location()
	dueDay := truncateDate(fact.DueAt.In(loc))
	// Calendar dates must not be subtracted as local instants across DST.
	civilDay := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	dayOffset := int(civilDay(businessDay).Sub(civilDay(dueDay)) / (24 * time.Hour))

	graceDays := int(fact.GracePeriodDays)

	switch {
	case dayOffset == -1:
		return windowedType(
			nowLocal,
			businessDay,
			PreDueHour,
			model.PreDueReminderType,
		)

	case dayOffset == 0:
		return windowedType(
			nowLocal,
			businessDay,
			DueTodayHour,
			model.DueTodayReminderType,
		)

	case dayOffset >= 1 && dayOffset <= graceDays:
		graceEndsAt := (model.Installment{DueAt: fact.DueAt, GracePeriodDays: fact.GracePeriodDays}).OverdueEligibleAt()
		// A delayed reminder must never call an expired duration-based grace
		// period active, even when the business calendar still calls it D+Grace.
		if !graceEndsAt.After(nowLocal) {
			return "", SuppressNoEligible
		}
		reminderType := model.GracePeriodReminderType
		if !graceEndsAt.After(businessDay.AddDate(0, 0, 1)) {
			reminderType = model.GraceFinalDayReminderType
		}

		return windowedType(
			nowLocal,
			businessDay,
			GraceHour,
			reminderType,
		)

	default:
		return "", SuppressNoEligible
	}
}

func windowedType(
	nowLocal time.Time,
	businessDay time.Time,
	hour int,
	reminderType model.ReminderType,
) (model.ReminderType, SuppressReason) {
	windowStart := time.Date(
		businessDay.Year(),
		businessDay.Month(),
		businessDay.Day(),
		hour,
		0,
		0,
		0,
		nowLocal.Location(),
	)
	// Delivery window ends at the start of the next business day so a delayed
	// job still sends same-day reminders, but never indefinitely.
	windowEnd := businessDay.AddDate(0, 0, 1)

	if nowLocal.Before(windowStart) {
		return "", SuppressBeforeWindow
	}

	if !nowLocal.Before(windowEnd) {
		return "", SuppressStaleWindow
	}

	return reminderType, SuppressNone
}

func templateFor(t model.ReminderType) string {
	switch t {
	case model.PreDueReminderType:
		return TemplatePreDue

	case model.DueTodayReminderType:
		return TemplateDueToday

	case model.GracePeriodReminderType:
		return TemplateGracePeriod

	case model.GraceFinalDayReminderType:
		return TemplateGraceFinalDay

	default:
		return ""
	}
}

func truncateDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// ValidateTerms documents the fixed v1 ceilings for wiring-time checks.
func ValidateTerms() error {
	const op = "repaymentreminder.v1.ValidateTerms"

	if MaxMessagesPerCustomerPerDay != 1 {
		return fmt.Errorf("%s: max messages per customer per day must be 1", op)
	}

	return nil
}
