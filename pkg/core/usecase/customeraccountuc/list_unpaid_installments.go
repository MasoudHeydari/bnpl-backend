package customeraccountuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	repaymentpriorityv1 "github.com/parstis/bnpl-backend/pkg/core/policy/repaymentpriority/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ListUnpaidInstallments returns the authenticated customer's unpaid installments
// (keyset-paginated) together with a customer-wide repayment summary.
//
// Unpaid means status scheduled, due, or overdue on a contract-attached row.
// An empty list is a successful result, including when no customer profile exists
// for the external subject ID.
//
// The repayment summary (counts, due installments, next_payable) is computed
// across all of the customer's unpaid obligations and is never page-scoped.
func (uc *UseCase) ListUnpaidInstallments(
	ctx context.Context,
	input ListUnpaidInstallmentsInput,
) (ListUnpaidInstallmentsOutput, error) {
	if input.ExternalCustomerID.IsZero() {
		return ListUnpaidInstallmentsOutput{}, externalCustomerIDRequiredError()
	}

	page := input.Page
	page.Order = pagination.OrderAsc
	page.Normalize()
	if err := page.Validate(); err != nil {
		return ListUnpaidInstallmentsOutput{}, invalidPaginationError(err)
	}

	fetchLimit := page.Limit + 1
	fetchPage := page
	fetchPage.Limit = fetchLimit

	now := uc.clock.Now().UTC()

	var (
		installments []UnpaidInstallment
		summary      RepaymentSummary
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.ReadSnapshot(ctx, func(ctx context.Context, tx repo.Tx) error {
				customerProfileQ := uc.customerProfile.Tx(tx)
				profile, err := customerProfileQ.GetByExternalCustomerID(ctx, input.ExternalCustomerID)
				if err != nil {
					if cerr.IsCode(err, cerr.CodeResourceNotFound) {
						installments = nil
						summary = emptyRepaymentSummary()
						return nil
					}

					return err
				}

				installmentQ := uc.installment.Tx(tx)
				paymentAttemptQ := uc.paymentAttempt.Tx(tx)
				installmentChargeQ := uc.installmentCharge.Tx(tx)

				rows, listErr := installmentQ.ListUnpaidByCustomerID(
					ctx,
					profile.ID,
					fetchPage,
				)
				if listErr != nil {
					return listErr
				}

				installments = make([]UnpaidInstallment, 0, len(rows))
				for _, row := range rows {
					item, mapErr := toUnpaidInstallment(row)
					if mapErr != nil {
						return mapErr
					}

					installments = append(installments, item)
				}

				built, summaryErr := buildRepaymentSummary(
					ctx,
					installmentQ,
					paymentAttemptQ,
					installmentChargeQ,
					profile.ID,
					now,
				)
				if summaryErr != nil {
					return summaryErr
				}

				summary = built
				return nil
			})
		},
	)
	if err != nil {
		return ListUnpaidInstallmentsOutput{}, err
	}

	hasNext := len(installments) > page.Limit
	if hasNext {
		installments = installments[:page.Limit]
	}

	meta := pagination.KeysetMeta{
		PaginationType: "keyset",
		Limit:          page.Limit,
		HasNext:        hasNext,
	}
	if hasNext && len(installments) > 0 {
		last := installments[len(installments)-1]
		afterID := last.ID.String()
		afterDueAt := last.DueAt.UTC().Format(time.RFC3339Nano)
		meta.NextAfterID = &afterID
		meta.NextAfterDueAt = &afterDueAt
	}

	return ListUnpaidInstallmentsOutput{
		Installments: installments,
		Repayment:    summary,
		Page:         meta,
	}, nil
}

func emptyRepaymentSummary() RepaymentSummary {
	return RepaymentSummary{
		DueInstallments:   []RepaymentDueInstallment{},
		DueCount:          0,
		UnpaidCount:       0,
		NextPayable:       nil,
		HasPendingPayment: false,
	}
}

func buildRepaymentSummary(
	ctx context.Context,
	installmentQ repo.InstallmentQueryer,
	paymentAttemptQ repo.PaymentAttemptQueryer,
	installmentChargeQ repo.InstallmentChargeQueryer,
	customerID model.CustomerID,
	now time.Time,
) (RepaymentSummary, error) {
	unpaidCount, countErr := installmentQ.CountUnpaidByCustomerID(ctx, customerID)
	if countErr != nil {
		return RepaymentSummary{}, countErr
	}

	dueRows, dueErr := installmentQ.ListDueUnpaidByCustomerID(ctx, customerID, now)
	if dueErr != nil {
		return RepaymentSummary{}, dueErr
	}

	nextRows, nextErr := installmentQ.ListNextPayablePerContractByCustomerID(ctx, customerID)
	if nextErr != nil {
		return RepaymentSummary{}, nextErr
	}

	candidateIDs := make([]model.InstallmentID, 0, len(nextRows)+len(dueRows))
	for _, row := range nextRows {
		candidateIDs = append(candidateIDs, row.ID)
	}

	for _, row := range dueRows {
		candidateIDs = append(candidateIDs, row.ID)
	}

	pendingIDs, pendingErr := paymentAttemptQ.ListInstallmentIDsWithNonTerminalAttempts(
		ctx,
		customerID,
		uniqueInstallmentIDs(candidateIDs),
	)
	if pendingErr != nil {
		return RepaymentSummary{}, pendingErr
	}

	pendingByID := make(map[model.InstallmentID]bool, len(pendingIDs))
	for _, id := range pendingIDs {
		pendingByID[id] = true
	}

	candidates := make([]repaymentpriorityv1.Candidate, 0, len(nextRows))
	for _, row := range nextRows {
		candidates = append(candidates, repaymentpriorityv1.Candidate{
			Installment:          row,
			HasUnresolvedPayment: pendingByID[row.ID],
		})
	}

	selection := repaymentpriorityv1.New().Select(candidates, now)

	dueViews := make([]RepaymentDueInstallment, 0, len(dueRows))
	for _, row := range dueRows {
		if row.ContractID == nil {
			continue
		}

		dueViews = append(dueViews, RepaymentDueInstallment{
			ID:                row.ID,
			ContractID:        *row.ContractID,
			InstallmentOrder:  row.InstallmentOrder,
			TotalAmountMinor:  row.TotalAmountMinor,
			Currency:          row.Currency,
			DueAt:             row.DueAt,
			Status:            row.Status,
			HasPendingPayment: pendingByID[row.ID],
		})
	}

	summary := RepaymentSummary{
		DueInstallments:   dueViews,
		DueCount:          len(dueViews),
		UnpaidCount:       unpaidCount,
		NextPayable:       nil,
		HasPendingPayment: selection.HasUnresolvedPayment,
	}

	if selection.Selected == nil || selection.Selected.ContractID == nil {
		return summary, nil
	}

	selected := *selection.Selected
	charges, chargesErr := installmentChargeQ.ListByInstallmentIDs(
		ctx,
		[]model.InstallmentID{selected.ID},
	)
	if chargesErr != nil {
		return RepaymentSummary{}, chargesErr
	}

	payableAmount, payableOK, payableErr := model.CustomerPayableAmount(selected, charges)
	if payableErr != nil {
		return RepaymentSummary{}, payableErr
	}

	var amountMinor uint64
	if payableOK && payableAmount > 0 && payableAmount == selected.PrincipalAmountMinor {
		amountMinor = payableAmount
	} else {
		// FC currently recognizes only principal. Do not advertise an amount the
		// payment command must reject because fee recognition is unavailable.
		payableOK = false
	}

	summary.NextPayable = &RepaymentNextPayable{
		InstallmentID:      selected.ID,
		ContractID:         *selected.ContractID,
		InstallmentOrder:   selected.InstallmentOrder,
		PayableAmountMinor: amountMinor,
		Currency:           selected.Currency,
		DueAt:              selected.DueAt,
		Status:             selected.Status,
		HasPendingPayment:  selection.HasUnresolvedPayment,
		PayableAmountKnown: payableOK,
	}

	return summary, nil
}

func uniqueInstallmentIDs(ids []model.InstallmentID) []model.InstallmentID {
	if len(ids) == 0 {
		return nil
	}

	seen := make(map[model.InstallmentID]struct{}, len(ids))
	out := make([]model.InstallmentID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		out = append(out, id)
	}

	return out
}

func toUnpaidInstallment(row model.Installment) (UnpaidInstallment, error) {
	const op = "customeraccountuc.toUnpaidInstallment"

	if row.ContractID == nil || row.ContractID.IsZero() {
		return UnpaidInstallment{}, cerr.InternalError(fmt.Errorf(
			"%s: unpaid installment %s is missing contract_id", op, row.ID,
		))
	}

	if !row.Status.IsActive() {
		return UnpaidInstallment{}, cerr.InternalError(fmt.Errorf(
			"%s: unexpected unpaid installment status in list query: %s", op, row.Status,
		))
	}

	return UnpaidInstallment{
		ID:                      row.ID,
		ContractID:              *row.ContractID,
		MerchantProfileID:       row.MerchantProfileID,
		MerchantBranchProfileID: row.MerchantBranchProfileID,
		InstallmentOrder:        row.InstallmentOrder,
		TotalAmountMinor:        row.TotalAmountMinor,
		Currency:                row.Currency,
		DueAt:                   row.DueAt,
		Status:                  row.Status,
		PaidAt:                  row.PaidAt,
	}, nil
}
