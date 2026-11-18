package installmentlifecycleuc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	collectionsv3 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v3"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// AssessLateFeesOutput reports late-fee assessment results for one job run.
type AssessLateFeesOutput struct {
	// AssessedInstallmentCount is the number of overdue installments inspected.
	AssessedInstallmentCount int64

	// CreatedChargeCount is the number of late_fee charges inserted.
	CreatedChargeCount int64

	// SkippedFrozenCount is the number of installments skipped because a claim
	// appeared after the SQL freeze filter (defensive).
	SkippedFrozenCount int64
}

// AssessLateFees creates one applied late_fee charge per overdue business day
// for claimed overdue installments. Fees are non-compounding
// (principal * 167 / 100000). Assessment freezes while a non-terminal payment
// attempt or repayment-batch claim is active so posted_applier payable
// snapshots cannot race newly assessed fees.
func (uc *UseCase) AssessLateFees(ctx context.Context) (AssessLateFeesOutput, error) {
	const op = "installmentlifecycleuc.UseCase.AssessLateFees"

	now := uc.clock.Now().UTC()
	loc := uc.clock.BusinessLocation()
	output := AssessLateFeesOutput{}

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					installmentQ := uc.installment.Tx(tx)
					chargeQ := uc.installmentCharge.Tx(tx)

					claimed, err := installmentQ.ClaimOverdueForLateFee(
						ctx,
						now,
						uc.AssessLateFeeBatchSize,
					)
					if err != nil {
						return fmt.Errorf("%s: claim overdue installments: %w", op, err)
					}

					if len(claimed) == 0 {
						return nil
					}

					ids := make([]model.InstallmentID, 0, len(claimed))
					for _, row := range claimed {
						ids = append(ids, row.ID)
					}

					existing, err := chargeQ.ListByInstallmentIDs(ctx, ids)
					if err != nil {
						return fmt.Errorf("%s: list existing charges: %w", op, err)
					}

					existingLateFeeDays := make(map[model.InstallmentID]map[string]struct{}, len(claimed))
					for _, charge := range existing {
						if charge.ChargeType != model.LateFeeInstallmentChargeType {
							continue
						}

						if charge.ChargeStatus == model.CanceledInstallmentChargeStatus {
							continue
						}

						if charge.DueAt == nil {
							continue
						}

						key := lateFeeBusinessDateKey(*charge.DueAt, loc)
						days, ok := existingLateFeeDays[charge.InstallmentID]
						if !ok {
							days = make(map[string]struct{})
							existingLateFeeDays[charge.InstallmentID] = days
						}

						days[key] = struct{}{}
					}

					for _, installmentRow := range claimed {
						output.AssessedInstallmentCount++

						if installmentRow.Status != model.OverdueInstallmentStatus {
							continue
						}

						amount := collectionsv3.LateFeeAmountMinor(installmentRow.PrincipalAmountMinor)
						if amount == 0 {
							continue
						}

						for _, dayStart := range overdueBusinessDaysThrough(installmentRow, now, loc) {
							key := lateFeeBusinessDateKey(dayStart, loc)
							if _, exists := existingLateFeeDays[installmentRow.ID][key]; exists {
								continue
							}

							dueAt := dayStart.UTC()
							metadata, metaErr := json.Marshal(map[string]any{
								"business_date":    key,
								"rate_numerator":   collectionsv3.LateFeeRateNumerator,
								"rate_denominator": collectionsv3.LateFeeRateDenominator,
							})
							if metaErr != nil {
								return cerr.InternalError(fmt.Errorf("%s: marshal late fee metadata: %w", op, metaErr))
							}

							_, createErr := chargeQ.Create(
								ctx,
								installmentRow.ID,
								model.LateFeeInstallmentChargeType,
								model.AppliedInstallmentChargeStatus,
								amount,
								installmentRow.Currency,
								model.CustomerInstallmentChargePayerType,
								model.PlatformInstallmentChargeRecipientType,
								collectionsv3.PolicyID,
								collectionsv3.PolicyVersion,
								now,
								&dueAt,
								nil,
								metadata,
								now,
								now,
							)
							if createErr != nil {
								if cerr.IsCode(createErr, cerr.CodeConflict) {
									output.SkippedFrozenCount++
									continue
								}

								return fmt.Errorf("%s: create late fee charge: %w", op, createErr)
							}

							output.CreatedChargeCount++
							days, ok := existingLateFeeDays[installmentRow.ID]
							if !ok {
								days = make(map[string]struct{})
								existingLateFeeDays[installmentRow.ID] = days
							}

							days[key] = struct{}{}
						}
					}

					return nil
				},
			)
		},
	)
	if err != nil {
		return AssessLateFeesOutput{}, err
	}

	if output.CreatedChargeCount > 0 {
		uc.observer.SweepCompleted(ctx, SweepAssessLateFees, output.CreatedChargeCount)
	}

	return output, nil
}

func lateFeeBusinessDateKey(at time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}

	local := at.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).Format("2006-01-02")
}

// overdueBusinessDaysThrough returns midnight (business location) for each
// overdue calendar day from OverdueEligibleAt through today's business date.
func overdueBusinessDaysThrough(
	installment model.Installment,
	now time.Time,
	loc *time.Location,
) []time.Time {
	if loc == nil {
		loc = time.UTC
	}

	overdueAt := installment.OverdueEligibleAt(loc)
	if now.Before(overdueAt) {
		return nil
	}

	startLocal := overdueAt.In(loc)
	start := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 0, 0, 0, 0, loc)

	nowLocal := now.In(loc)
	end := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)

	days := make([]time.Time, 0)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		days = append(days, day)
	}

	return days
}
