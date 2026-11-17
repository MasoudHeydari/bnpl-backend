package checkout

import (
	"fmt"
	"math"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/calendar/jalali"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CadenceUnit identifies the calendar arithmetic used between recurring
// installment due dates.
type CadenceUnit string

const (
	// CadenceCalendarDay advances due dates by whole civil calendar days in the
	// schedule anchor's location (not a fixed 24h duration multiply).
	CadenceCalendarDay CadenceUnit = "calendar_day"

	// CadenceJalaliMonth advances due dates by Jalali months with month-end
	// clamping via jalali.AddMonthsClamped.
	CadenceJalaliMonth CadenceUnit = "jalali_month"
)

// IsValid reports whether the cadence unit is a known enumeration value.
func (u CadenceUnit) IsValid() bool {
	switch u {
	case CadenceCalendarDay, CadenceJalaliMonth:
		return true

	default:
		return false
	}
}

// RepaymentCadence is the immutable spacing between consecutive recurring
// installment due dates.
type RepaymentCadence struct {
	// Unit selects calendar-day or Jalali-month arithmetic.
	Unit CadenceUnit

	// Every is the positive interval count in Unit (for example Every=5 with
	// CadenceCalendarDay means every five calendar days).
	Every uint
}

// ReminderTerms are the checkout-policy-owned notification windows for
// recurring installments. Each eligible installment is notified independently.
// There is no customer-day aggregate cap.
type ReminderTerms struct {
	// LeadDays is how many calendar days before due date ordinary Pre-Due
	// reminders may begin (D-LeadDays .. D-1).
	LeadDays uint
}

// RepaymentTerms is the cohesive repayment contract owned by a Checkout Policy
// version: cadence, grace, and reminder eligibility.
type RepaymentTerms struct {
	// Cadence spaces recurring installment due dates.
	Cadence RepaymentCadence

	// GracePeriodDays is snapshotted onto each installment at offer time.
	GracePeriodDays uint

	// Reminder governs Pre-Due lead time and the customer-day SMS ceiling.
	Reminder ReminderTerms
}

// MinimumRecurringCadenceCalendarDays is the smallest allowed spacing between
// consecutive recurring installment due dates.
const MinimumRecurringCadenceCalendarDays = 4

// AddCalendarDays advances t by days on the civil calendar in t's location,
// preserving wall-clock fields. Prefer this over multiplying time.Duration by
// 24h when contractual day boundaries must survive timezone transitions.
func AddCalendarDays(t time.Time, days int) time.Time {
	loc := t.Location()
	local := t.In(loc)

	return time.Date(
		local.Year(),
		local.Month(),
		local.Day()+days,
		local.Hour(),
		local.Minute(),
		local.Second(),
		local.Nanosecond(),
		loc,
	)
}

// AddCadenceIntervals advances anchor by intervals complete cadence steps.
// intervals must be non-negative. Each call recomputes from the original
// anchor so Jalali month-end clamping cannot accumulate drift.
func AddCadenceIntervals(anchor time.Time, cadence RepaymentCadence, intervals int) (time.Time, error) {
	const op = "checkout.AddCadenceIntervals"

	if intervals < 0 {
		return time.Time{}, fmt.Errorf("%s: intervals must be non-negative", op)
	}

	if cadence.Every == 0 {
		return time.Time{}, fmt.Errorf("%s: cadence every must be positive", op)
	}

	if !cadence.Unit.IsValid() {
		return time.Time{}, fmt.Errorf("%s: unknown cadence unit %q", op, cadence.Unit)
	}

	if intervals == 0 {
		return anchor, nil
	}

	switch cadence.Unit {
	case CadenceCalendarDay:
		step, err := checkedMulUint(cadence.Every, uint(intervals))
		if err != nil {
			return time.Time{}, fmt.Errorf("%s: calendar-day step overflow: %w", op, err)
		}

		return AddCalendarDays(anchor, int(step)), nil

	case CadenceJalaliMonth:
		step, err := checkedMulUint(cadence.Every, uint(intervals))
		if err != nil {
			return time.Time{}, fmt.Errorf("%s: jalali-month step overflow: %w", op, err)
		}

		return jalali.AddMonthsClamped(anchor, int(step)), nil

	default:
		return time.Time{}, fmt.Errorf("%s: unknown cadence unit %q", op, cadence.Unit)
	}
}

func checkedMulUint(a, b uint) (uint, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}

	product := a * b
	if product/a != b {
		return 0, fmt.Errorf("uint multiply overflow")
	}

	if product > uint(math.MaxInt) {
		return 0, fmt.Errorf("cadence step exceeds supported int range")
	}

	return product, nil
}

// CivilCalendarDaysBetween returns the signed whole-day difference between the
// civil calendar dates of a and b in loc (b - a).
func CivilCalendarDaysBetween(a, b time.Time, loc *time.Location) int {
	if loc == nil {
		loc = time.UTC
	}

	civil := func(t time.Time) time.Time {
		local := t.In(loc)
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	}

	return int(civil(b).Sub(civil(a)) / (24 * time.Hour))
}

// DueAtForInstallmentOrder computes the due instant for installment order
// (1-based) from schedule anchor now using the policy terms.
//
// ActivationAfterInitialPayment: order 1 is the checkout payment window from
// now; orders 2..n are recurring at cadence intervals 1..(n-1) from now.
// ActivationAfterMerchantFunding: every installment is recurring at cadence
// intervals equal to its order (first due after one full cadence).
func DueAtForInstallmentOrder(terms Terms, now time.Time, order int) (time.Time, error) {
	const op = "checkout.DueAtForInstallmentOrder"

	if order <= 0 {
		return time.Time{}, fmt.Errorf("%s: installment order must be positive", op)
	}

	switch terms.ActivationMode {
	case ActivationAfterInitialPayment:
		if order == 1 {
			return now.Add(terms.FirstInstallmentPaymentWindow), nil
		}

		return AddCadenceIntervals(now, terms.Repayment.Cadence, order-1)

	case ActivationAfterMerchantFunding:
		return AddCadenceIntervals(now, terms.Repayment.Cadence, order)

	default:
		return time.Time{}, fmt.Errorf("%s: unknown activation mode %q", op, terms.ActivationMode)
	}
}

// MinimumRecurringSpacingDays returns the smallest civil-day gap between
// consecutive recurring due dates under terms. Calendar-day cadence uses Every
// directly; Jalali-month cadence samples anchors that stress month lengths.
func MinimumRecurringSpacingDays(terms Terms) (int, error) {
	const op = "checkout.MinimumRecurringSpacingDays"

	cadence := terms.Repayment.Cadence
	if !cadence.Unit.IsValid() {
		return 0, fmt.Errorf("%s: unknown cadence unit %q", op, cadence.Unit)
	}

	if cadence.Every == 0 {
		return 0, fmt.Errorf("%s: cadence every must be positive", op)
	}

	switch cadence.Unit {
	case CadenceCalendarDay:
		return int(cadence.Every), nil

	case CadenceJalaliMonth:
		return minimumJalaliRecurringSpacingDays(cadence)

	default:
		return 0, fmt.Errorf("%s: unknown cadence unit %q", op, cadence.Unit)
	}
}

func minimumJalaliRecurringSpacingDays(cadence RepaymentCadence) (int, error) {
	const op = "checkout.minimumJalaliRecurringSpacingDays"

	// Sample anchors covering 31-day months, Esfand leap/non-leap, and year
	// boundaries so clamping cannot hide a sub-minimum gap.
	anchors := []time.Time{
		time.Date(2024, time.March, 19, 12, 0, 0, 0, time.UTC), // 29 Esfand 1402 (non-leap)
		time.Date(2024, time.March, 20, 12, 0, 0, 0, time.UTC), // 1 Farvardin 1403
		time.Date(2024, time.April, 19, 12, 0, 0, 0, time.UTC), // 31 Farvardin 1403
		time.Date(2025, time.March, 20, 12, 0, 0, 0, time.UTC), // 30 Esfand 1403 (leap)
		time.Date(2025, time.September, 21, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 20, 12, 0, 0, 0, time.UTC),
	}

	minGap := int(^uint(0) >> 1)
	for _, anchor := range anchors {
		prev, err := AddCadenceIntervals(anchor, cadence, 1)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", op, err)
		}

		for interval := 2; interval <= 14; interval++ {
			next, addErr := AddCadenceIntervals(anchor, cadence, interval)
			if addErr != nil {
				return 0, fmt.Errorf("%s: %w", op, addErr)
			}

			gap := CivilCalendarDaysBetween(prev, next, time.UTC)
			if gap <= 0 {
				return 0, fmt.Errorf("%s: non-increasing jalali schedule at interval %d", op, interval)
			}

			if gap < minGap {
				minGap = gap
			}

			prev = next
		}
	}

	return minGap, nil
}

// ValidateTerms fail-fast checks a policy's Terms for bootstrap registration.
func ValidateTerms(terms Terms) error {
	const op = "checkout.ValidateTerms"

	if terms.InstallmentCount <= 0 {
		return fmt.Errorf("%s: installment count must be positive", op)
	}

	if terms.DecisionTTL <= 0 {
		return fmt.Errorf("%s: decision ttl must be positive", op)
	}

	if terms.MaxFinancedAmountMinor == 0 {
		return fmt.Errorf("%s: max financed amount must be positive", op)
	}

	switch terms.FinancingMode {
	case PartialWithUpfrontFinancing, FullFinancingOnly:
	default:
		return fmt.Errorf("%s: unknown financing mode %q", op, terms.FinancingMode)
	}

	switch terms.ActivationMode {
	case ActivationAfterInitialPayment, ActivationAfterMerchantFunding:
	default:
		return fmt.Errorf("%s: unknown activation mode %q", op, terms.ActivationMode)
	}

	switch terms.ScheduleAnchor {
	case ScheduleFromDecision, ScheduleFromContractActivation:
	default:
		return fmt.Errorf("%s: unknown schedule anchor %q", op, terms.ScheduleAnchor)
	}

	if terms.ActivationMode == ActivationAfterInitialPayment {
		if terms.FirstInstallmentPaymentWindow <= 0 {
			return fmt.Errorf("%s: initial-payment activation requires a positive first installment payment window", op)
		}

		if terms.ScheduleAnchor != ScheduleFromContractActivation {
			// Recurring dues must finalize at activation; the checkout payment
			// deadline alone may still be decision-anchored inside the schedule
			// builder for installment #1.
			return fmt.Errorf("%s: initial-payment activation requires schedule anchor %q so recurring dues finalize at activation", op, ScheduleFromContractActivation)
		}
	}

	if terms.ActivationMode == ActivationAfterMerchantFunding {
		if terms.FirstInstallmentPaymentWindow != 0 {
			return fmt.Errorf("%s: merchant-funding activation must not set a first installment payment window", op)
		}

		if terms.ScheduleAnchor != ScheduleFromContractActivation {
			return fmt.Errorf("%s: merchant-funding activation requires schedule anchor %q", op, ScheduleFromContractActivation)
		}
	}

	if err := ValidateRepaymentTerms(terms.Repayment); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	spacing, err := MinimumRecurringSpacingDays(terms)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if spacing < MinimumRecurringCadenceCalendarDays {
		return fmt.Errorf(
			"%s: recurring cadence spacing %d calendar days is below minimum %d",
			op,
			spacing,
			MinimumRecurringCadenceCalendarDays,
		)
	}

	grace := terms.Repayment.GracePeriodDays
	if grace > model.MaxInstallmentGracePeriodDays {
		return fmt.Errorf(
			"%s: grace period days %d exceeds operational maximum %d",
			op,
			grace,
			model.MaxInstallmentGracePeriodDays,
		)
	}

	// G < C-1  ⇒  G <= C-2 so default eligibility for installment n begins
	// strictly before installment n+1 is due.
	if int(grace) >= spacing-1 {
		return fmt.Errorf(
			"%s: grace period days %d must be strictly less than cadence spacing %d minus one",
			op,
			grace,
			spacing,
		)
	}

	lead := terms.Repayment.Reminder.LeadDays
	if lead == 0 {
		return fmt.Errorf("%s: reminder lead days must be positive", op)
	}

	if int(lead) >= spacing {
		return fmt.Errorf(
			"%s: reminder lead days %d must be strictly less than cadence spacing %d",
			op,
			lead,
			spacing,
		)
	}

	return nil
}

// ValidateRepaymentTerms checks cadence shape without activation/schedule coupling.
func ValidateRepaymentTerms(terms RepaymentTerms) error {
	const op = "checkout.ValidateRepaymentTerms"

	if !terms.Cadence.Unit.IsValid() {
		return fmt.Errorf("%s: unknown cadence unit %q", op, terms.Cadence.Unit)
	}

	if terms.Cadence.Every == 0 {
		return fmt.Errorf("%s: cadence every must be positive", op)
	}

	if terms.Cadence.Unit == CadenceCalendarDay && terms.Cadence.Every < MinimumRecurringCadenceCalendarDays {
		return fmt.Errorf(
			"%s: calendar-day cadence every must be at least %d, got %d",
			op,
			MinimumRecurringCadenceCalendarDays,
			terms.Cadence.Every,
		)
	}

	return nil
}

// ValidateGeneratedSchedule checks a concrete installment schedule against the
// policy terms that produced it. Invalid schedules must not be persisted.
//
// Exact cadence: each recurring DueAt must equal DueAtForInstallmentOrder from
// the schedule's original activation/decision anchor (never chained from a
// previously clamped due date). Grace must satisfy G < C-1 against the actual
// recurring calendar spacing C, with C >= MinimumRecurringCadenceCalendarDays.
//
// schedule.Anchor is the authoritative generation instant. Callers validating
// persisted drafts must supply the same anchor used at generation time.
func ValidateGeneratedSchedule(terms Terms, schedule ScheduleResult, financedAmountMinor, upfrontAmountMinor uint64) error {
	const op = "checkout.ValidateGeneratedSchedule"

	if err := ValidateTerms(terms); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if len(schedule.Installments) == 0 {
		return fmt.Errorf("%s: schedule must contain at least one installment", op)
	}

	if len(schedule.Installments) != terms.InstallmentCount {
		return fmt.Errorf(
			"%s: installment count %d does not equal policy terms count %d",
			op,
			len(schedule.Installments),
			terms.InstallmentCount,
		)
	}

	if schedule.Anchor.IsZero() {
		return fmt.Errorf("%s: schedule anchor is required for exact cadence validation", op)
	}

	anchor := schedule.Anchor
	grace := int(terms.Repayment.GracePeriodDays)
	if grace < 0 {
		return fmt.Errorf("%s: grace period days must be non-negative", op)
	}

	var principalSum uint64
	var prevDue time.Time
	var recurringSpacing *int

	for i, draft := range schedule.Installments {
		if draft.Order != i+1 {
			return fmt.Errorf("%s: installment order must be contiguous starting at 1", op)
		}

		if draft.DueAt.IsZero() {
			return fmt.Errorf("%s: installment %d due_at must be set", op, draft.Order)
		}

		expectedDue, err := DueAtForInstallmentOrder(terms, anchor, draft.Order)
		if err != nil {
			return fmt.Errorf("%s: expected due for installment %d: %w", op, draft.Order, err)
		}

		if !draft.DueAt.Equal(expectedDue) {
			return fmt.Errorf(
				"%s: installment %d due_at %s does not equal expected cadence instant %s",
				op,
				draft.Order,
				draft.DueAt.UTC().Format(time.RFC3339Nano),
				expectedDue.UTC().Format(time.RFC3339Nano),
			)
		}

		expectedTotal, err := model.SumAmountMinor(draft.PrincipalAmountMinor, draft.UpfrontAmountMinor, draft.BaseFeeAmountMinor)
		if err != nil {
			return fmt.Errorf("%s: installment %d total: %w", op, draft.Order, err)
		}

		if draft.TotalAmountMinor != expectedTotal {
			return fmt.Errorf("%s: installment %d total does not equal principal+upfront+fee", op, draft.Order)
		}

		principalSum, err = model.AddAmountMinor(principalSum, draft.PrincipalAmountMinor)
		if err != nil {
			return fmt.Errorf("%s: principal sum overflow: %w", op, err)
		}

		if i > 0 {
			if !draft.DueAt.After(prevDue) {
				return fmt.Errorf("%s: installment due dates must be strictly increasing", op)
			}

			// Recurring spacing excludes the normal-customer initial checkout
			// payment deadline (order 1 under ActivationAfterInitialPayment).
			if !(terms.ActivationMode == ActivationAfterInitialPayment && i == 1) {
				gap := CivilCalendarDaysBetween(prevDue, draft.DueAt, draft.DueAt.Location())
				if gap < MinimumRecurringCadenceCalendarDays {
					return fmt.Errorf(
						"%s: installment %d spacing %d calendar days is below minimum %d",
						op,
						draft.Order,
						gap,
						MinimumRecurringCadenceCalendarDays,
					)
				}

				// Every recurring pair is checked. Jalali month lengths vary, so
				// the first gap is not a substitute for later gaps.
				if grace >= gap-1 {
					return fmt.Errorf(
						"%s: grace period days %d must be strictly less than installment %d spacing %d minus 1",
						op,
						grace,
						draft.Order,
						gap,
					)
				}

				if recurringSpacing == nil {
					recurringSpacing = &gap
				} else if gap != *recurringSpacing && terms.Repayment.Cadence.Unit == CadenceCalendarDay {
					// Calendar-day cadence must keep a constant civil spacing.
					// Jalali months intentionally vary by month length / clamping.
					return fmt.Errorf(
						"%s: installment %d calendar-day spacing %d differs from earlier recurring spacing %d",
						op,
						draft.Order,
						gap,
						*recurringSpacing,
					)
				}
			}
		}

		prevDue = draft.DueAt

		if terms.ActivationMode == ActivationAfterInitialPayment {
			if draft.Order == 1 {
				if draft.UpfrontAmountMinor != upfrontAmountMinor {
					return fmt.Errorf("%s: first installment must carry the full upfront amount", op)
				}
			} else if draft.UpfrontAmountMinor != 0 {
				return fmt.Errorf("%s: recurring installment %d must not carry upfront", op, draft.Order)
			}
		}

		if terms.ActivationMode == ActivationAfterMerchantFunding && draft.UpfrontAmountMinor != 0 {
			return fmt.Errorf("%s: merchant-funding schedules must not include upfront", op)
		}
	}

	if principalSum != financedAmountMinor {
		return fmt.Errorf("%s: principal sum %d does not equal financed amount %d", op, principalSum, financedAmountMinor)
	}

	return nil
}
