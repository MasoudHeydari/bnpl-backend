package contractuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// CreateContractInput carries inputs for creating a checkout contract.
//
// ExternalCustomerID is the authenticated Identity Service subject ID
// (x-auth-subject-id / customer_profiles.external_customer_id). Ownership checks use the
// resolved customer_profiles.id against checkout_decision_allocations.customer_id.
//
// ProviderCode is optional. When empty, Financial Core may resolve provider/payment
// policy for payment-based activation. Merchant-funding policies ignore it.
type CreateContractInput struct {
	// DecisionID identifies the decision.
	DecisionID model.DecisionID
	// ExternalCustomerID is the Identity Service subject mapped to customer_profiles.external_customer_id.
	ExternalCustomerID model.ExternalCustomerID

	// TenantID identifies the tenant.
	TenantID model.TenantID
	// ContextID identifies the payment context.
	ContextID model.ContextID

	// ProviderCode optionally selects a Financial Core payment provider.
	// Empty means FC may apply its default provider/payment policy. Unused when
	// the Decision policy activates without an initial payment.
	ProviderCode string
}

// CreateContractOutput returns the result of generic checkout contract creation.
//
// Depending on the Decision policy ActivationMode, Contract starts as
// payment_pending in both cases: waiting for the customer's initial payment
// (PaymentRequired true) or for Financial Core merchant funding
// (PaymentRequired false). It becomes active only after the relevant Financial
// Core success. Payment fields are populated only when PaymentRequired is true.
type CreateContractOutput struct {
	// Result reports the use-case outcome classification.
	Result CreateContractResult

	// Contract is the created or idempotently reused contract.
	Contract model.Contract
	// PaymentRequired reports whether this policy requires an initial checkout payment before activation.
	PaymentRequired bool
	// PaymentAttempt is the created or replayed payment attempt when PaymentRequired is true.
	PaymentAttempt model.PaymentAttempt
	// Payment is the payment session projection when PaymentRequired is true.
	Payment CheckoutPaymentOutput
}

// CreateContract creates a contract according to the exact checkout policy
// snapshot persisted on its Decision. Policy selection is never re-run here:
// changing a customer's runtime assignment only affects future Decisions.
func (uc *UseCase) CreateContract(
	ctx context.Context,
	input CreateContractInput,
) (CreateContractOutput, error) {
	return uc.createContract(ctx, input, true)
}

// PrepareCheckoutContract persists the Contract and PaymentAttempt without invoking
// Financial Core. CheckoutSession must durably link the prepared Contract before
// submitting payment: authoritative results may arrive immediately after submit.
func (uc *UseCase) PrepareCheckoutContract(ctx context.Context, input CreateContractInput) (CreateContractOutput, error) {
	return uc.createContract(ctx, input, false)
}

// SubmitPreparedCheckoutFulfillment is called after CheckoutSession has
// committed its contract link. FC calls and collection obligations remain
// outside that TX.
//
// Authority is the Decision's persisted Checkout Policy ActivationMode — not
// prepared.PaymentRequired. That field is a projection that must agree with
// the policy; a mismatch fails closed as an internal invariant error so a
// buggy caller cannot switch monetary semantics.
//
//   - ActivationAfterInitialPayment: ensures the PaymentAttempt and Financial
//     Core payment session; the contract activates after the initial payment
//     succeeds.
//   - ActivationAfterMerchantFunding: creates NO PaymentAttempt. Records
//     merchant entitlement with Financial Core (collected cash 0) and
//     activates the contract only after that Financial Core success.
func (uc *UseCase) SubmitPreparedCheckoutFulfillment(ctx context.Context, prepared CreateContractOutput) (CreateContractOutput, error) {
	const op = "contractuc.UseCase.SubmitPreparedCheckoutFulfillment"

	mode, err := uc.resolveContractActivationMode(ctx, prepared.Contract)
	if err != nil {
		return CreateContractOutput{}, err
	}

	policyRequiresPayment := mode == checkout.ActivationAfterInitialPayment
	if prepared.PaymentRequired != policyRequiresPayment {
		return CreateContractOutput{}, cerr.InternalError(fmt.Errorf(
			"%s: prepared.PaymentRequired=%v disagrees with Decision ActivationMode %q",
			op,
			prepared.PaymentRequired,
			mode,
		))
	}

	switch mode {
	case checkout.ActivationAfterInitialPayment:
		contract, attempt, payment, err := uc.ensureCheckoutPaymentSession(ctx, prepared.Contract, prepared.PaymentAttempt)
		if err != nil {
			return CreateContractOutput{}, err
		}

		prepared.Contract = contract
		prepared.PaymentAttempt = attempt
		prepared.Payment = payment

		if prepared.Result == CreateContractResultCreated {
			uc.businessMetrics.ContractCreated(ctx, string(prepared.Contract.Status))
		}

		return prepared, nil

	case checkout.ActivationAfterMerchantFunding:
		// payment_pending waits for FC merchant funding; an already-active
		// contract replays EnsureMerchantEntitlement idempotently.
		if prepared.Contract.Status == model.PaymentPendingContractStatus ||
			prepared.Contract.Status == model.ActiveContractStatus {
			return uc.submitPreparedMerchantFunding(ctx, prepared)
		}

		return prepared, nil

	default:
		return CreateContractOutput{}, cerr.InternalError(fmt.Errorf(
			"%s: unsupported contract activation mode %q",
			op,
			mode,
		))
	}
}

// resolveContractActivationMode loads the immutable Decision policy snapshot
// that owns this contract's fulfillment strategy.
func (uc *UseCase) resolveContractActivationMode(
	ctx context.Context,
	contract model.Contract,
) (checkout.ActivationMode, error) {
	const op = "contractuc.UseCase.resolveContractActivationMode"

	var selected checkout.Policy

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		decisionQ := uc.decision.Conn(conn)
		decision, err := decisionQ.GetByID(ctx, contract.DecisionID)
		if err != nil {
			return err
		}

		var resolveErr error
		selected, resolveErr = uc.checkoutPolicies.Resolve(policy.Ref{
			ID:      decision.PolicyID,
			Version: decision.PolicyVersion,
		})
		if resolveErr != nil {
			return fmt.Errorf("%s: resolve decision checkout policy: %w", op, resolveErr)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return selected.Terms().ActivationMode, nil
}

func (uc *UseCase) createContract(ctx context.Context, input CreateContractInput, submitPayment bool) (CreateContractOutput, error) {
	const op = "contractuc.UseCase.createContract"

	if err := validateCreateContractInput(input); err != nil {
		return CreateContractOutput{}, err
	}

	var selected checkout.Policy
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		decisionQ := uc.decision.Conn(conn)
		decision, err := decisionQ.GetByID(ctx, input.DecisionID)
		if err != nil {
			return err
		}

		selected, err = uc.checkoutPolicies.Resolve(policy.Ref{
			ID:      decision.PolicyID,
			Version: decision.PolicyVersion,
		})
		if err != nil {
			return fmt.Errorf("%s: resolve decision checkout policy: %w", op, err)
		}

		return nil
	})
	if err != nil {
		return CreateContractOutput{}, err
	}

	switch selected.Terms().ActivationMode {
	case checkout.ActivationAfterInitialPayment:
		return uc.createPaymentPending(ctx, input, submitPayment)

	case checkout.ActivationAfterMerchantFunding:
		return uc.createMerchantFundingContract(ctx, input, selected, submitPayment)

	default:
		return CreateContractOutput{}, fmt.Errorf(
			"%s: unsupported contract activation mode %q",
			op,
			selected.Terms().ActivationMode,
		)
	}
}

func buildContractPlan(
	selected checkout.Policy,
	allocation model.CheckoutDecisionAllocation,
	first model.Installment,
) (checkout.ContractPlan, error) {
	return selected.BuildContractPlan(checkout.ContractPlanInput{
		RequestedAmountMinor:                 allocation.RequestedAmountMinor,
		FinancedAmountMinor:                  allocation.ApprovedFinancedAmountMinor,
		UpfrontAmountMinor:                   allocation.UpfrontAmountMinor,
		FirstInstallmentPrincipalAmountMinor: first.PrincipalAmountMinor,
		FirstInstallmentTotalAmountMinor:     first.TotalAmountMinor,
	})
}
