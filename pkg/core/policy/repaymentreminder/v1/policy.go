package v1

import (
	"fmt"
	"sort"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	// PolicyVersion identifies this repayment reminder policy revision.
	PolicyVersion = "repayment_reminder.v1"

	// PreDueHour is the local hour for Pre-Due reminders (D-LeadDays .. D-1).
	PreDueHour = 10

	// DueTodayHour is the local hour for D0 reminders.
	DueTodayHour = 10

	// GraceHour is the local hour for grace-period reminders (D+1..D+Grace).
	GraceHour = 13

	// TemplatePreDue is the Notification Service template for Pre-Due reminders.
	TemplatePreDue = "bnpl.repayment.pre_due.v1"

	// TemplateDueToday is the Notification Service template for D0 reminders.
	TemplateDueToday = "bnpl.repayment.due_today.v1"

	// TemplateGracePeriod is the Notification Service template for mid-grace reminders.
	TemplateGracePeriod = "bnpl.repayment.grace_period.v1"

	// TemplateGraceFinalDay is the Notification Service template for final grace day.
	TemplateGraceFinalDay = "bnpl.repayment.grace_final_day.v1"
)

// SuppressReason explains why a single installment was not eligible.
type SuppressReason string

// Known suppression reasons (bounded, safe for metrics labels).
const (
	SuppressNone           SuppressReason = ""
	SuppressNoEligible     SuppressReason = "no_eligible"
	SuppressBeforeWindow   SuppressReason = "before_window"
	SuppressStaleWindow    SuppressReason = "stale_window"
	SuppressPendingPayment SuppressReason = "pending_payment"
	SuppressNotUnpaid      SuppressReason = "not_unpaid"
	SuppressAlreadySent    SuppressReason = "already_sent"
)

// InstallmentFact carries the explicit facts needed to evaluate one installment.
type InstallmentFact struct {
	// InstallmentID identifies the installment.
	InstallmentID model.InstallmentID

	// DueAt is the contractual due instant (interpreted in business location).
	DueAt time.Time

	// GracePeriodDays is the contractual grace snapshot from the installment.
	GracePeriodDays uint

	// LeadDays is the Pre-Due lead window from the originating Checkout Policy
	// repayment terms. Discovery may be broader than this value; eligibility
	// must use the installment's own lead.
	LeadDays uint

	// Status is the current installment lifecycle status.
	Status model.InstallmentStatus

	// HasUnresolvedPayment is true when a non-terminal payment attempt exists.
	HasUnresolvedPayment bool
}

// NotificationDecision is one independently eligible installment notification.
//
// Aggregation is forbidden: each eligible installment/event produces its own
// durable SMS request. Priority orders processing when several decisions are
// eligible in the same dispatch; it never suppresses another eligible decision.
type NotificationDecision struct {
	// InstallmentID is the single installment this notification describes.
	InstallmentID model.InstallmentID

	// ReminderType is the semantic reminder class for this installment.
	ReminderType model.ReminderType

	// TemplateID is the Notification Service template.
	TemplateID string

	// BusinessDate is the calendar date in the business timezone.
	BusinessDate time.Time

	// PolicyVersion identifies this policy revision.
	PolicyVersion string

	// EpisodeID is set only for collections Default notifications.
	EpisodeID *model.DefaultEpisodeID
}

// Policy is the v1 repayment reminder policy.
type Policy struct{}

// New returns the v1 repayment reminder policy.
func New() Policy {
	return Policy{}
}

// EvaluateAll returns every independently eligible ordinary reminder decision
// for the supplied installment facts on the given business calendar day.
//
// Candidate discovery (repository due-date window) is intentionally broader
// than eligibility: discovery may include installments outside today's send
// window so a delayed job can still catch same-day slots, while eligibility
// applies hour windows, grace endings, and pending-payment suppression per
// installment. Multiple decisions for one customer/day are required — a Due
// Today for contract B must not suppress a Grace Final for contract A.
//
// Results are sorted by ReminderType priority (descending) then InstallmentID
// for deterministic processing order only.
func (Policy) EvaluateAll(
	now time.Time,
	businessDate time.Time,
	facts []InstallmentFact,
) []NotificationDecision {
	loc := now.Location()
	if loc == nil {
		loc = time.UTC
	}

	businessDay := truncateDate(businessDate.In(loc))
	nowLocal := now.In(loc)

	decisions := make([]NotificationDecision, 0, len(facts))
	for _, fact := range facts {
		slot, reason := evaluateInstallment(nowLocal, businessDay, fact)
		if reason != SuppressNone || slot == "" {
			continue
		}

		decisions = append(decisions, NotificationDecision{
			InstallmentID: fact.InstallmentID,
			ReminderType:  slot,
			TemplateID:    templateFor(slot),
			BusinessDate:  businessDay,
			PolicyVersion: PolicyVersion,
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

	return decisions
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
	leadDays := int(fact.LeadDays)

	switch {
	case leadDays > 0 && dayOffset >= -leadDays && dayOffset <= -1:
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
		graceEndsAt := (model.Installment{DueAt: fact.DueAt, GracePeriodDays: fact.GracePeriodDays}).OverdueEligibleAt(loc)
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

// NotificationIdempotencyKey returns the durable semantic business identity for
// an ordinary repayment reminder. Redispatch and concurrent workers collide on
// this key instead of a customer-day quota.
func NotificationIdempotencyKey(
	installmentID model.InstallmentID,
	reminderType model.ReminderType,
	businessDate time.Time,
) string {
	day := businessDate.UTC().Format("2006-01-02")
	if loc := businessDate.Location(); loc != nil {
		day = businessDate.In(loc).Format("2006-01-02")
	}

	return fmt.Sprintf("repayment:%s:%s:%s", installmentID.String(), reminderType, day)
}

// DefaultNotificationIdempotencyKey returns the durable identity for one
// Default notice tied to a specific installment inside an episode. Multiple
// triggering installments must never share one SMS.
func DefaultNotificationIdempotencyKey(
	episodeID model.DefaultEpisodeID,
	installmentID model.InstallmentID,
) string {
	return fmt.Sprintf("default:%s:%s", episodeID.String(), installmentID.String())
}

// ValidateTerms documents wiring-time checks for this policy revision.
func ValidateTerms() error {
	return nil
}
