package customeraccountuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ListPayments returns the authenticated customer's finalized installment payment history,
// newest first.
//
// Only payment attempts in states succeeded, failed, or canceled with purposes
// checkout_first_installment or installment_repayment are included.
// An empty list is a successful result, including when no customer profile exists
// for the external subject ID.
func (uc *UseCase) ListPayments(ctx context.Context, input ListPaymentsInput) (ListPaymentsOutput, error) {
	if input.ExternalCustomerID.IsZero() {
		return ListPaymentsOutput{}, externalCustomerIDRequiredError()
	}

	page := input.Page
	page.Order = pagination.OrderDesc
	page.Normalize()
	if err := page.Validate(); err != nil {
		return ListPaymentsOutput{}, invalidPaginationError(err)
	}

	fetchLimit := page.Limit + 1
	fetchPage := page
	fetchPage.Limit = fetchLimit

	var payments []Payment

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			customerProfileQ := uc.customerProfile.Conn(conn)
			profile, err := customerProfileQ.GetByExternalCustomerID(ctx, input.ExternalCustomerID)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					payments = nil
					return nil
				}

				return err
			}

			paymentAttemptQ := uc.paymentAttempt.Conn(conn)
			rows, listErr := paymentAttemptQ.ListFinalizedInstallmentPaymentsByCustomerID(
				ctx,
				profile.ID,
				fetchPage,
			)
			if listErr != nil {
				return listErr
			}

			payments = make([]Payment, 0, len(rows))
			for _, row := range rows {
				payment, mapErr := toPayment(row)
				if mapErr != nil {
					return mapErr
				}

				payments = append(payments, payment)
			}

			return nil
		},
	)
	if err != nil {
		return ListPaymentsOutput{}, err
	}

	hasNext := len(payments) > page.Limit
	if hasNext {
		payments = payments[:page.Limit]
	}

	meta := pagination.KeysetMeta{
		PaginationType: "keyset",
		Limit:          page.Limit,
		HasNext:        hasNext,
	}
	if hasNext && len(payments) > 0 {
		last := payments[len(payments)-1]
		afterID := last.ID.String()
		afterCreatedAt := last.CreatedAt.UTC().Format(time.RFC3339Nano)
		meta.NextAfterID = &afterID
		meta.NextAfterCreatedAt = &afterCreatedAt
	}

	return ListPaymentsOutput{
		Payments: payments,
		Page:     meta,
	}, nil
}

func toPayment(row model.PaymentAttempt) (Payment, error) {
	const op = "customeraccountuc.toPayment"

	switch row.State {
	case model.SucceededPaymentAttemptState,
		model.FailedPaymentAttemptState,
		model.CanceledPaymentAttemptState:

	default:
		return Payment{}, cerr.InternalError(fmt.Errorf(
			"%s: unexpected payment attempt state in history query: %s", op, row.State,
		))
	}

	switch row.Purpose {

	case model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		model.InstallmentRepaymentFinancialCorePaymentPurpose:

	default:
		return Payment{}, cerr.InternalError(fmt.Errorf(
			"%s: unexpected payment attempt purpose in history query: %s", op, row.Purpose,
		))
	}

	return Payment{
		ID:            row.ID,
		ContractID:    row.ContractID,
		InstallmentID: row.InstallmentID,
		AmountMinor:   row.AmountMinor,
		Currency:      row.Currency,
		Status:        row.State,
		Purpose:       row.Purpose,
		CreatedAt:     row.CreatedAt,
		CompletedAt:   completedAt(row),
	}, nil
}

// completedAt returns the state-specific finalization timestamp when present.
// succeeded -> applied_at; failed -> failed_at; canceled -> canceled_at.
func completedAt(row model.PaymentAttempt) *time.Time {
	switch row.State {
	case model.SucceededPaymentAttemptState:
		return row.AppliedAt

	case model.FailedPaymentAttemptState:
		return row.FailedAt

	case model.CanceledPaymentAttemptState:
		return row.CanceledAt

	default:
		return nil
	}
}
