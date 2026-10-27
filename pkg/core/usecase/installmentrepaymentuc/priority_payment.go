package installmentrepaymentuc

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// CreatePriorityPaymentAttemptInput starts repayment for the server-selected
// priority installment of the authenticated customer.
//
// The browser must not supply an installment id for the reminder priority flow.
type CreatePriorityPaymentAttemptInput struct {
	// TenantID identifies the tenant.
	TenantID model.TenantID
	// ContextID identifies the payment context.
	ContextID model.ContextID

	// ExternalCustomerID is the Identity Service subject mapped to
	// customer_profiles.external_customer_id.
	ExternalCustomerID model.ExternalCustomerID

	// IdempotencyKey is the client idempotency key for the operation.
	IdempotencyKey string
	// Channel is the payment initiation channel.
	Channel model.PaymentChannel
	// ProviderCode optionally selects the Financial Core checkout payment provider.
	ProviderCode string
}

// CreatePriorityPaymentAttemptOutput reports priority repayment initiation.
type CreatePriorityPaymentAttemptOutput struct {
	// Result reports the use-case outcome classification.
	Result CreatePaymentAttemptResult
	// PaymentAttempt is the created or replayed payment attempt.
	PaymentAttempt model.PaymentAttempt
	// ContractID identifies the contract of the selected installment.
	ContractID model.ContractID
	// InstallmentID identifies the server-selected priority installment.
	InstallmentID model.InstallmentID
}

// CreatePriorityPaymentAttempt selects the current priority installment and
// reuses CreatePaymentAttempt so sequencing, FC authority, and idempotency stay
// in one place.
func (uc *UseCase) CreatePriorityPaymentAttempt(
	ctx context.Context,
	input CreatePriorityPaymentAttemptInput,
) (CreatePriorityPaymentAttemptOutput, error) {
	const op = "installmentrepaymentuc.UseCase.CreatePriorityPaymentAttempt"

	if input.ExternalCustomerID.IsZero() {
		return CreatePriorityPaymentAttemptOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: external customer id is required", op),
		)
	}

	if input.IdempotencyKey == "" {
		return CreatePriorityPaymentAttemptOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: idempotency key is required", op),
		)
	}

	now := uc.clock.Now().UTC()

	var selected *model.Installment

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		customerProfileQ := uc.customerProfile.Conn(conn)
		profile, err := customerProfileQ.GetByExternalCustomerID(ctx, input.ExternalCustomerID)
		if err != nil {
			return err
		}

		installmentQ := uc.installment.Conn(conn)
		page := pagination.KeysetRequest{Limit: 200, Order: pagination.OrderAsc}
		page.Normalize()

		rows, listErr := installmentQ.ListUnpaidByCustomerID(ctx, profile.ID, page)
		if listErr != nil {
			return listErr
		}

		selected = selectPriorityInstallment(rows, now)
		return nil
	})
	if err != nil {
		return CreatePriorityPaymentAttemptOutput{}, err
	}

	if selected == nil || selected.ContractID == nil {
		return CreatePriorityPaymentAttemptOutput{}, cerr.ResourceNotFound(
			fmt.Errorf("%s: no payable installment", op),
		)
	}

	out, createErr := uc.CreatePaymentAttempt(ctx, CreatePaymentAttemptInput{
		TenantID:           input.TenantID,
		ContextID:          input.ContextID,
		ContractID:         *selected.ContractID,
		InstallmentID:      selected.ID,
		ExternalCustomerID: input.ExternalCustomerID,
		IdempotencyKey:     input.IdempotencyKey,
		Channel:            input.Channel,
		ProviderCode:       input.ProviderCode,
	})
	if createErr != nil {
		return CreatePriorityPaymentAttemptOutput{}, createErr
	}

	return CreatePriorityPaymentAttemptOutput{
		Result:         out.Result,
		PaymentAttempt: out.PaymentAttempt,
		ContractID:     *selected.ContractID,
		InstallmentID:  selected.ID,
	}, nil
}

func selectPriorityInstallment(
	installments []model.Installment,
	now time.Time,
) *model.Installment {
	grouped := make(map[model.ContractID][]model.Installment)
	for _, row := range installments {
		if row.ContractID == nil {
			continue
		}

		id := *row.ContractID
		grouped[id] = append(grouped[id], row)
	}

	type candidate struct {
		installment model.Installment
		pastDue     bool
	}

	var candidates []candidate
	for _, rows := range grouped {
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
