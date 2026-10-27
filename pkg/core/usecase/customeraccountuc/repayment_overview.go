package customeraccountuc

import (
	"context"
	"sort"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// GetRepaymentOverviewInput identifies the authenticated external customer.
type GetRepaymentOverviewInput struct {
	// ExternalCustomerID is the Identity Service subject mapped to
	// customer_profiles.external_customer_id.
	ExternalCustomerID model.ExternalCustomerID
}

// RepaymentOverviewInstallment is one unpaid installment in the repayment overview.
type RepaymentOverviewInstallment struct {
	// ID uniquely identifies the installment.
	ID model.InstallmentID
	// ContractID identifies the contract.
	ContractID model.ContractID
	// InstallmentOrder is the 1-based order in the contract schedule.
	InstallmentOrder int
	// TotalAmountMinor is the payable amount in minor units.
	TotalAmountMinor uint64
	// Currency is the ISO currency.
	Currency model.Currency
	// DueAt is when the installment becomes due.
	DueAt time.Time
	// Status is the current lifecycle status.
	Status model.InstallmentStatus
	// IsPriority is true when this installment is the server-selected priority target.
	IsPriority bool
	// IsPayableInSequence is true when this installment is next payable in its contract.
	IsPayableInSequence bool
	// HasPendingPayment is true when a non-terminal payment attempt exists.
	HasPendingPayment bool
}

// PriorityRepaymentTarget identifies the installment the reminder flow should pay.
type PriorityRepaymentTarget struct {
	// InstallmentID identifies the priority installment.
	InstallmentID model.InstallmentID
	// ContractID identifies the owning contract.
	ContractID model.ContractID
	// DueAt is when the installment became or becomes due.
	DueAt time.Time
	// TotalAmountMinor is the payable amount in minor units.
	TotalAmountMinor uint64
	// Currency is the ISO currency.
	Currency model.Currency
	// Status is the current lifecycle status.
	Status model.InstallmentStatus
}

// GetRepaymentOverviewOutput carries unpaid installments and the priority target.
type GetRepaymentOverviewOutput struct {
	// Installments lists unpaid installments ordered by due_at ASC, id ASC.
	Installments []RepaymentOverviewInstallment
	// Priority is the server-selected priority repayment target when one exists.
	Priority *PriorityRepaymentTarget
}

// GetRepaymentOverview returns unpaid installments and the current priority target.
//
// Priority is the earliest past-due installment that is currently next-payable
// under per-contract sequencing. When no past-due payable installment remains,
// Priority is the earliest next-payable scheduled installment across contracts.
func (uc *UseCase) GetRepaymentOverview(
	ctx context.Context,
	input GetRepaymentOverviewInput,
) (GetRepaymentOverviewOutput, error) {
	if input.ExternalCustomerID.IsZero() {
		return GetRepaymentOverviewOutput{}, externalCustomerIDRequiredError()
	}

	now := uc.clock.Now().UTC()

	var (
		installments []model.Installment
		pendingIDs   map[model.InstallmentID]bool
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		customerProfileQ := uc.customerProfile.Conn(conn)
		profile, err := customerProfileQ.GetByExternalCustomerID(ctx, input.ExternalCustomerID)
		if err != nil {
			if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				installments = nil
				return nil
			}

			return err
		}

		installmentQ := uc.installment.Conn(conn)
		page := pagination.KeysetRequest{Limit: 200, Order: pagination.OrderAsc}
		page.Normalize()

		rows, listErr := installmentQ.ListUnpaidByCustomerID(ctx, profile.ID, page)
		if listErr != nil {
			return listErr
		}

		installments = rows

		// Pending payment detection uses reminder-candidate enrichment when the
		// due window overlaps; fall back to false when no candidate join is available
		// on this read path. Overview still surfaces unpaid state correctly.
		pendingIDs = map[model.InstallmentID]bool{}
		if len(rows) > 0 {
			dueFrom := rows[0].DueAt.AddDate(0, 0, -int(model.MaxInstallmentGracePeriodDays)-1)
			dueTo := rows[len(rows)-1].DueAt.AddDate(0, 0, 2)
			candidates, candErr := installmentQ.ListReminderCandidates(
				ctx,
				dueFrom,
				dueTo,
				len(rows)*2,
			)
			if candErr != nil {
				return candErr
			}

			for _, candidate := range candidates {
				if candidate.Installment.CustomerID == profile.ID {
					pendingIDs[candidate.Installment.ID] = candidate.HasUnresolvedPayment
				}
			}
		}

		return nil
	})
	if err != nil {
		return GetRepaymentOverviewOutput{}, err
	}

	priority := selectPriorityInstallment(installments, now)
	views := make([]RepaymentOverviewInstallment, 0, len(installments))

	byContract := groupInstallmentsByContract(installments)
	for _, row := range installments {
		if row.ContractID == nil {
			continue
		}

		contractRows := byContract[*row.ContractID]
		isNext := model.IsNextPayableInSequence(row, contractRows)
		isPriority := priority != nil && priority.ID == row.ID

		views = append(views, RepaymentOverviewInstallment{
			ID:                  row.ID,
			ContractID:          *row.ContractID,
			InstallmentOrder:    row.InstallmentOrder,
			TotalAmountMinor:    row.TotalAmountMinor,
			Currency:            row.Currency,
			DueAt:               row.DueAt,
			Status:              row.Status,
			IsPriority:          isPriority,
			IsPayableInSequence: isNext,
			HasPendingPayment:   pendingIDs[row.ID],
		})
	}

	var priorityView *PriorityRepaymentTarget
	if priority != nil && priority.ContractID != nil {
		priorityView = &PriorityRepaymentTarget{
			InstallmentID:    priority.ID,
			ContractID:       *priority.ContractID,
			DueAt:            priority.DueAt,
			TotalAmountMinor: priority.TotalAmountMinor,
			Currency:         priority.Currency,
			Status:           priority.Status,
		}
	}

	return GetRepaymentOverviewOutput{
		Installments: views,
		Priority:     priorityView,
	}, nil
}

func groupInstallmentsByContract(
	installments []model.Installment,
) map[model.ContractID][]model.Installment {
	grouped := make(map[model.ContractID][]model.Installment)
	for _, row := range installments {
		if row.ContractID == nil {
			continue
		}

		id := *row.ContractID
		grouped[id] = append(grouped[id], row)
	}

	return grouped
}

// selectPriorityInstallment chooses the earliest past-due next-payable installment
// across contracts. When none are past due, it returns the earliest next-payable
// installment by due_at, then id.
func selectPriorityInstallment(
	installments []model.Installment,
	now time.Time,
) *model.Installment {
	byContract := groupInstallmentsByContract(installments)

	type candidate struct {
		installment model.Installment
		pastDue     bool
	}

	var candidates []candidate
	for _, rows := range byContract {
		next, ok := model.NextPayableInSequence(rows)
		if !ok || next.ContractID == nil {
			continue
		}

		pastDue := !next.DueAt.After(now) ||
			next.Status == model.DueInstallmentStatus ||
			next.Status == model.OverdueInstallmentStatus

		candidates = append(candidates, candidate{
			installment: next,
			pastDue:     pastDue,
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.pastDue != b.pastDue {
			return a.pastDue
		}

		if !a.installment.DueAt.Equal(b.installment.DueAt) {
			return a.installment.DueAt.Before(b.installment.DueAt)
		}

		return a.installment.ID.String() < b.installment.ID.String()
	})

	selected := candidates[0].installment
	return &selected
}
