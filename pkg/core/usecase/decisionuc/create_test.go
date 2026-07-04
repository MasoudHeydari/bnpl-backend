package decisionuc_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	checkoutv1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

func testRequestContext() context.Context {
	ctx := context.Background()
	ctx = trace.WithTraceID(ctx, "trace-test")
	ctx = trace.WithCorrelationID(ctx, "corr-test")
	return ctx
}

type createDeps struct {
	pool                    *mock.Pool
	conn                    *mock.Conn
	tx                      *mock.Tx
	decision                *mock.Decision
	decisionTx              *mock.DecisionTxQueryer
	customerProfile         *mock.CustomerProfile
	customerProfileTx       *mock.CustomerProfileTxQueryer
	creditAccount           *mock.CreditAccount
	creditAccountTx         *mock.CreditAccountTxQueryer
	merchantProfile         *mock.MerchantProfile
	merchantProfileTx       *mock.MerchantProfileTxQueryer
	merchantBranchProfile   *mock.MerchantBranchProfile
	merchantBranchProfileTx *mock.MerchantBranchProfileTxQueryer
	allocation              *mock.CheckoutDecisionAllocation
	allocationTx            *mock.CheckoutDecisionAllocationTxQueryer
	policyAssignment        *mock.PolicyAssignment
	policyAssignmentTx      *mock.PolicyAssignmentTxQueryer
	installment             *mock.Installment
	installmentTx           *mock.InstallmentTxQueryer
	installmentCharge       *mock.InstallmentCharge
	installmentChargeTx     *mock.InstallmentChargeTxQueryer
	outbox                  *mock.Outbox
	outboxTx                *mock.OutboxTxQueryer
	idGenerator             *mock.IdentifierGenerator
	clock                   adapterclock.Fixed
	jurisdictions           domaincapability.OriginationJurisdictions
}

func newCreateDeps(t *testing.T, now time.Time) createDeps {
	t.Helper()

	return createDeps{
		pool:                    mock.NewPool(t),
		conn:                    mock.NewConn(t),
		tx:                      mock.NewTx(t),
		decision:                mock.NewDecision(t),
		decisionTx:              mock.NewDecisionTxQueryer(t),
		customerProfile:         mock.NewCustomerProfile(t),
		customerProfileTx:       mock.NewCustomerProfileTxQueryer(t),
		creditAccount:           mock.NewCreditAccount(t),
		creditAccountTx:         mock.NewCreditAccountTxQueryer(t),
		merchantProfile:         mock.NewMerchantProfile(t),
		merchantProfileTx:       mock.NewMerchantProfileTxQueryer(t),
		merchantBranchProfile:   mock.NewMerchantBranchProfile(t),
		merchantBranchProfileTx: mock.NewMerchantBranchProfileTxQueryer(t),
		allocation:              mock.NewCheckoutDecisionAllocation(t),
		allocationTx:            mock.NewCheckoutDecisionAllocationTxQueryer(t),
		policyAssignment:        mock.NewPolicyAssignment(t),
		policyAssignmentTx:      mock.NewPolicyAssignmentTxQueryer(t),
		installment:             mock.NewInstallment(t),
		installmentTx:           mock.NewInstallmentTxQueryer(t),
		installmentCharge:       mock.NewInstallmentCharge(t),
		installmentChargeTx:     mock.NewInstallmentChargeTxQueryer(t),
		outbox:                  mock.NewOutbox(t),
		outboxTx:                mock.NewOutboxTxQueryer(t),
		idGenerator:             mock.NewIdentifierGenerator(t),
		clock:                   adapterclock.NewFixed(now, time.UTC),
	}
}

func (d createDeps) newUseCase(t *testing.T) *decisionuc.UseCase {
	t.Helper()

	registry, err := checkoutcatalog.NewRegistry()
	if err != nil {
		panic(err)
	}

	jurisdictions := d.jurisdictions
	if jurisdictions == nil {
		jurisdictions = mustTestJurisdictions(t)
	}

	uc, err := decisionuc.New(
		d.pool,
		d.decision,
		d.customerProfile,
		d.creditAccount,
		d.merchantProfile,
		d.merchantBranchProfile,
		d.allocation,
		d.policyAssignment,
		d.installment,
		d.installmentCharge,
		d.outbox,
		d.clock,
		sha256fingerprint.New(),
		registry,
		checkoutcatalog.DefaultRef(),
		"bnpl.audit.events.v1",
		d.idGenerator,
		jurisdictions,
		metricsnoop.New(),
		decisionuc.AllowAllNewCreditUsage{},
	)
	if err != nil {
		panic(err)
	}

	return uc
}

func (d createDeps) expectDefaultCheckoutPolicyAssignment(ctx context.Context) {
	d.policyAssignmentTx.EXPECT().GetActive(
		ctx,
		model.CustomerPolicySubjectType,
		testifymock.Anything,
		model.CheckoutPolicyFamily,
		testifymock.Anything,
	).Return(model.PolicyAssignment{}, cerr.ResourceNotFound(errors.New("missing"))).Once()
}

func (d createDeps) expectConnTx(ctx context.Context) {
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
	d.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, d.tx)
		},
	).Once()
}

func (d createDeps) expectTxBindings() {
	d.decision.EXPECT().Tx(d.tx).Return(d.decisionTx).Once()
	d.allocation.EXPECT().Tx(d.tx).Return(d.allocationTx).Once()
	d.customerProfile.EXPECT().Tx(d.tx).Return(d.customerProfileTx).Once()
	d.creditAccount.EXPECT().Tx(d.tx).Return(d.creditAccountTx).Once()
	d.merchantProfile.EXPECT().Tx(d.tx).Return(d.merchantProfileTx).Once()
	d.merchantBranchProfile.EXPECT().Tx(d.tx).Return(d.merchantBranchProfileTx).Once()
	d.policyAssignment.EXPECT().Tx(d.tx).Return(d.policyAssignmentTx).Once()
	d.installment.EXPECT().Tx(d.tx).Return(d.installmentTx).Once()
	d.installmentCharge.EXPECT().Tx(d.tx).Return(d.installmentChargeTx).Once()
	d.outbox.EXPECT().Tx(d.tx).Return(d.outboxTx).Once()
}

func (d createDeps) expectAuditOutbox(
	t *testing.T,
	ctx context.Context,
	eventID string,
	decisionID model.DecisionID,
	externalCustomerID model.ExternalCustomerID,
	outcome audit.Outcome,
) *audit.Event {
	t.Helper()

	captured := new(audit.Event)
	d.idGenerator.EXPECT().Generate().Return(eventID).Once()
	d.outboxTx.EXPECT().Create(
		ctx,
		testifymock.MatchedBy(func(message repo.CreateOutboxMessage) bool {
			if message.Destination != "bnpl.audit.events.v1" ||
				message.MessageKey != decisionID.String() ||
				message.EventType != audit.EventType {
				return false
			}

			var event audit.Event
			if err := json.Unmarshal(message.Payload, &event); err != nil {
				return false
			}

			if event.Data.Actor.Type != audit.CustomerActorType ||
				event.Data.Actor.ID == nil ||
				*event.Data.Actor.ID != externalCustomerID.String() ||
				event.Data.Actor.Service != nil {
				return false
			}

			if event.TraceID != trace.ID(ctx) ||
				event.CorrelationID != trace.CorrelationID(ctx) {
				return false
			}

			if event.EventID != eventID ||
				event.Data.Action != "checkout_decision.create" ||
				event.Data.Resource.ID != decisionID.String() ||
				event.Data.Outcome != outcome {
				return false
			}

			if err := audit.ValidateEvent(event); err != nil {
				return false
			}

			*captured = event
			return true
		}),
	).Return(model.OutboxMessage{}, nil).Once()

	return captured
}

func eligibleFixtures(now time.Time) (
	externalCustomerID model.ExternalCustomerID,
	posGpsID model.PosGpsID,
	customerProfile model.CustomerProfile,
	merchantProfile model.MerchantProfile,
	branch model.MerchantBranchProfile,
	creditAccount model.CreditAccount,
) {
	externalCustomerID = model.ExternalCustomerID(uuid.New())
	posGpsID = model.PosGpsID(uuid.New())
	merchantIdentityID := model.MerchantID(uuid.New())

	customerProfile = model.CustomerProfile{
		ID:                 model.CustomerID(uuid.New()),
		ExternalCustomerID: externalCustomerID,
		Jurisdiction:       model.IRJurisdiction,
		Status:             model.ActiveCustomerStatus,
	}
	merchantProfile = model.MerchantProfile{
		ID:                model.MerchantProfileID(uuid.New()),
		MerchantID:        merchantIdentityID,
		Status:            model.ActiveMerchantStatus,
		Currencies:        []model.Currency{model.IRRCurrency},
		MaxAmountMinor:    1_000_000,
		MaxAmountCurrency: model.IRRCurrency,
		MaxTenureMonths:   12,
		Jurisdiction:      model.IRJurisdiction,
	}
	branch = model.MerchantBranchProfile{
		ID:         model.MerchantBranchProfileID(uuid.New()),
		MerchantID: merchantIdentityID,
		BranchID:   model.BranchID(uuid.New()),
		Type:       model.PhysicalMerchantBranchType,
		Status:     model.ActiveMerchantBranchStatus,
	}
	expiresAt := now.Add(365 * 24 * time.Hour)
	creditAccount = model.CreditAccount{
		LimitAmountMinor: 500_000,
		Currency:         model.IRRCurrency,
		Status:           model.ActiveCreditAccountStatus,
		ExpiresAt:        &expiresAt,
	}

	return externalCustomerID, posGpsID, customerProfile, merchantProfile, branch, creditAccount
}

func expectOfferedInstallments(
	t *testing.T,
	deps createDeps,
	ctx context.Context,
	decisionID model.DecisionID,
	allocation model.CheckoutDecisionAllocation,
	now time.Time,
) []model.Installment {
	t.Helper()

	installments := make([]model.Installment, 0, allocation.InstallmentCount)
	for order := 1; order <= allocation.InstallmentCount; order++ {
		row := model.Installment{
			ID:                      model.InstallmentID(uuid.New()),
			DecisionID:              decisionID,
			CustomerID:              allocation.CustomerID,
			MerchantProfileID:       allocation.MerchantProfileID,
			MerchantBranchProfileID: allocation.MerchantBranchProfileID,
			InstallmentOrder:        order,
			Currency:                allocation.Currency,
			PrincipalAmountMinor:    25_000,
			UpfrontAmountMinor:      0,
			BaseFeeAmountMinor:      0,
			TotalAmountMinor:        25_000,
			DueAt:                   now.Add(time.Duration(order) * time.Hour),
			Status:                  model.OfferedInstallmentStatus,
		}

		if order == 1 {
			row.PrincipalAmountMinor = 25_000
			row.TotalAmountMinor = 25_000
		}

		deps.installmentTx.EXPECT().Create(
			ctx,
			decisionID,
			(*model.ContractID)(nil),
			allocation.CustomerID,
			allocation.MerchantProfileID,
			allocation.MerchantBranchProfileID,
			order,
			allocation.Currency,
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			model.OfferedInstallmentStatus,
			uint64(0),
			(*time.Time)(nil),
			now,
			now,
		).Return(row, nil).Once()
		installments = append(installments, row)
	}

	return installments
}

func TestCreate_NilBranchIdentity_Rejected(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCreateDeps(t, now)

	_, err := deps.newUseCase(t).Create(ctx, decisionuc.CreateInput{
		CheckoutKey:        "checkout-key",
		ExternalCustomerID: model.ExternalCustomerID(uuid.New()),
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: model.PosGpsID{},
		},
		RequestedAmountMinor: 100_000,
		Currency:             model.IRRCurrency,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestCreate_MissingBranch_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCreateDeps(t, now)
	externalCustomerID, posGpsID, customerProfile, _, _, _ := eligibleFixtures(now)

	deps.expectConnTx(ctx)
	deps.expectTxBindings()
	deps.expectDefaultCheckoutPolicyAssignment(ctx)
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, testifymock.Anything).
		Return(model.Decision{}, cerr.ResourceNotFound(errors.New("missing"))).Once()
	deps.customerProfileTx.EXPECT().LockByExternalCustomerID(ctx, externalCustomerID).Return(customerProfile, nil).Once()
	deps.merchantBranchProfileTx.EXPECT().LockByPosGpsID(ctx, posGpsID).
		Return(model.MerchantBranchProfile{}, cerr.ResourceNotFound(errors.New("branch missing"))).Once()

	_, err := deps.newUseCase(t).Create(ctx, decisionuc.CreateInput{
		CheckoutKey:        "checkout-key",
		ExternalCustomerID: externalCustomerID,
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: posGpsID,
		},
		RequestedAmountMinor: 100_000,
		Currency:             model.IRRCurrency,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeResourceNotFound))
}

func TestCreate_Denied_NoInstallments_ReturnsReasonCodes(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCreateDeps(t, now)
	externalCustomerID, posGpsID, customerProfile, merchantProfile, branch, _ := eligibleFixtures(now)
	branch.Status = model.SuspendedMerchantBranchStatus
	decisionID := model.DecisionID(uuid.New())
	expiresAt := now.Add(checkoutv1.New().Terms().DecisionTTL)

	deps.expectConnTx(ctx)
	deps.expectTxBindings()
	deps.expectDefaultCheckoutPolicyAssignment(ctx)
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, testifymock.Anything).
		Return(model.Decision{}, cerr.ResourceNotFound(errors.New("missing"))).Once()
	deps.customerProfileTx.EXPECT().LockByExternalCustomerID(ctx, externalCustomerID).Return(customerProfile, nil).Once()
	deps.merchantBranchProfileTx.EXPECT().LockByPosGpsID(ctx, posGpsID).Return(branch, nil).Once()
	deps.merchantProfileTx.EXPECT().LockByMerchantID(ctx, branch.MerchantID).Return(merchantProfile, nil).Once()
	deps.creditAccountTx.EXPECT().GetByCustomerIDAndCurrency(
		ctx, customerProfile.ID, model.IRRCurrency,
	).Return(model.CreditAccount{}, cerr.ResourceNotFound(errors.New("no credit account"))).Once()

	deps.decisionTx.EXPECT().Create(
		ctx,
		model.ContractApprovalDecisionType,
		model.CheckoutKeyDecisionTargetType,
		testifymock.Anything,
		model.DeniedDecisionOutcome,
		[]string{model.MerchantBranchProfileNotActiveDecisionReason},
		"checkout_contract_approval",
		"v1",
		model.IRJurisdiction,
		model.SystemDecisionActorType,
		(*uuid.UUID)(nil),
		(*model.DecisionID)(nil),
		(*time.Time)(nil),
		testifymock.Anything,
		now,
		now,
		testifymock.Anything,
		testifymock.Anything,
	).Return(model.Decision{
		ID:             decisionID,
		Type:           model.ContractApprovalDecisionType,
		Outcome:        model.DeniedDecisionOutcome,
		ReasonCodes:    []string{model.MerchantBranchProfileNotActiveDecisionReason},
		PolicyID:       "checkout_contract_approval",
		PolicyVersion:  "v1",
		ActorType:      model.SystemDecisionActorType,
		IdempotencyKey: "checkout-key",
		ExpiresAt:      &expiresAt,
	}, nil).Once()
	deps.allocationTx.EXPECT().Create(
		ctx,
		decisionID,
		"checkout-key",
		customerProfile.ID,
		merchantProfile.ID,
		&branch.ID,
		model.POSPurchaseChannel,
		posGpsID.UUID(),
		(*string)(nil),
		uint64(100_000),
		uint64(0),
		uint64(0),
		uint64(0),
		model.IRRCurrency,
		4,
		expiresAt,
		now,
	).Return(model.CheckoutDecisionAllocation{
		DecisionID:              decisionID,
		CheckoutKey:             "checkout-key",
		CustomerID:              customerProfile.ID,
		MerchantProfileID:       merchantProfile.ID,
		MerchantBranchProfileID: &branch.ID,
		PurchaseChannel:         model.POSPurchaseChannel,
		PurchaseSourceID:        posGpsID.UUID(),
		RequestedAmountMinor:    100_000,
		AvailableCreditMinor:    0,
		Currency:                model.IRRCurrency,
		InstallmentCount:        4,
		ExpiresAt:               expiresAt,
	}, nil).Once()
	auditEvent := deps.expectAuditOutbox(
		t,
		ctx,
		"audit-denied",
		decisionID,
		externalCustomerID,
		audit.DeniedOutcome,
	)

	output, err := deps.newUseCase(t).Create(ctx, decisionuc.CreateInput{
		CheckoutKey:        "checkout-key",
		ExternalCustomerID: externalCustomerID,
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: posGpsID,
		},
		RequestedAmountMinor: 100_000,
		Currency:             model.IRRCurrency,
	})
	require.NoError(t, err)
	require.Equal(t, model.DeniedDecisionOutcome, output.Decision.Outcome)
	require.Equal(t, []string{model.MerchantBranchProfileNotActiveDecisionReason}, output.Decision.ReasonCodes)
	require.Equal(t, uint64(100_000), output.RequestedAmountMinor)
	require.Equal(t, uint64(0), output.AvailableCreditMinor)
	require.Equal(t, model.IRRCurrency, output.Currency)
	require.Nil(t, output.InstallmentOffer)

	require.Equal(t, model.MerchantBranchProfileNotActiveDecisionReason, *auditEvent.Data.ReasonCode)
	require.Equal(t, []any{model.MerchantBranchProfileNotActiveDecisionReason}, auditEvent.Data.Metadata["reason_codes"])
	require.Equal(t, customerProfile.ID.String(), auditEvent.Data.Metadata["customer_id"])
	require.Equal(t, externalCustomerID.String(), auditEvent.Data.Metadata["identity_subject_id"])
	require.Equal(t, merchantProfile.ID.String(), auditEvent.Data.Metadata["merchant_profile_id"])
	require.Equal(t, merchantProfile.MerchantID.String(), auditEvent.Data.Metadata["merchant_id"])
	require.Equal(t, branch.ID.String(), auditEvent.Data.Metadata["merchant_branch_profile_id"])
	require.Equal(t, branch.BranchID.String(), auditEvent.Data.Metadata["branch_id"])
	require.Equal(t, string(model.SystemDecisionActorType), fmt.Sprint(auditEvent.Data.Metadata["decision_actor_type"]))
	_, hasInstallmentIDs := auditEvent.Data.Metadata["installment_ids"]
	require.False(t, hasInstallmentIDs)
	_, hasInstallmentCount := auditEvent.Data.Metadata["installment_count"]
	require.False(t, hasInstallmentCount)
	for _, change := range auditEvent.Data.Changes {
		require.NotEqual(t, "installments.status", change.Field)
	}
}

func TestCreate_Approved_CreatesInstallmentsAndAudit(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCreateDeps(t, now)
	externalCustomerID, posGpsID, customerProfile, merchantProfile, branch, creditAccount := eligibleFixtures(now)
	decisionID := model.DecisionID(uuid.New())
	expiresAt := now.Add(checkoutv1.New().Terms().DecisionTTL)

	deps.expectConnTx(ctx)
	deps.expectTxBindings()
	deps.expectDefaultCheckoutPolicyAssignment(ctx)
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, testifymock.Anything).
		Return(model.Decision{}, cerr.ResourceNotFound(errors.New("missing"))).Once()
	deps.customerProfileTx.EXPECT().LockByExternalCustomerID(ctx, externalCustomerID).Return(customerProfile, nil).Once()
	deps.merchantBranchProfileTx.EXPECT().LockByPosGpsID(ctx, posGpsID).Return(branch, nil).Once()
	deps.merchantProfileTx.EXPECT().LockByMerchantID(ctx, branch.MerchantID).Return(merchantProfile, nil).Once()
	deps.creditAccountTx.EXPECT().GetByCustomerIDAndCurrency(
		ctx, customerProfile.ID, model.IRRCurrency,
	).Return(creditAccount, nil).Once()

	deps.decisionTx.EXPECT().Create(
		ctx,
		model.ContractApprovalDecisionType,
		model.CheckoutKeyDecisionTargetType,
		testifymock.Anything,
		model.ApprovedDecisionOutcome,
		[]string{model.ApprovedFullFinancingDecisionReason},
		"checkout_contract_approval",
		"v1",
		model.IRJurisdiction,
		model.SystemDecisionActorType,
		(*uuid.UUID)(nil),
		(*model.DecisionID)(nil),
		(*time.Time)(nil),
		testifymock.Anything,
		now,
		now,
		testifymock.Anything,
		testifymock.Anything,
	).Return(model.Decision{
		ID:             decisionID,
		Type:           model.ContractApprovalDecisionType,
		Outcome:        model.ApprovedDecisionOutcome,
		ReasonCodes:    []string{model.ApprovedFullFinancingDecisionReason},
		PolicyID:       "checkout_contract_approval",
		PolicyVersion:  "v1",
		ActorType:      model.SystemDecisionActorType,
		IdempotencyKey: "checkout-key",
		ExpiresAt:      &expiresAt,
	}, nil).Once()

	allocation := model.CheckoutDecisionAllocation{
		DecisionID:                  decisionID,
		CheckoutKey:                 "checkout-key",
		CustomerID:                  customerProfile.ID,
		MerchantProfileID:           merchantProfile.ID,
		MerchantBranchProfileID:     &branch.ID,
		RequestedAmountMinor:        100_000,
		ApprovedFinancedAmountMinor: 100_000,
		UpfrontAmountMinor:          0,
		AvailableCreditMinor:        500_000,
		Currency:                    model.IRRCurrency,
		InstallmentCount:            4,
		ExpiresAt:                   expiresAt,
	}
	deps.allocationTx.EXPECT().Create(
		ctx,
		decisionID,
		"checkout-key",
		customerProfile.ID,
		merchantProfile.ID,
		&branch.ID,
		model.POSPurchaseChannel,
		posGpsID.UUID(),
		(*string)(nil),
		uint64(100_000),
		uint64(100_000),
		uint64(0),
		uint64(500_000),
		model.IRRCurrency,
		4,
		testifymock.Anything,
		now,
	).Return(allocation, nil).Once()

	installments := expectOfferedInstallments(
		t,
		deps,
		ctx,
		decisionID,
		allocation,
		now,
	)
	auditEvent := deps.expectAuditOutbox(
		t,
		ctx,
		"audit-approved",
		decisionID,
		externalCustomerID,
		audit.SucceededOutcome,
	)

	output, err := deps.newUseCase(t).Create(ctx, decisionuc.CreateInput{
		CheckoutKey:        "checkout-key",
		ExternalCustomerID: externalCustomerID,
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: posGpsID,
		},
		RequestedAmountMinor: 100_000,
		Currency:             model.IRRCurrency,
	})
	require.NoError(t, err)
	require.Equal(t, decisionuc.CreateResultCreated, output.Result)
	require.Equal(t, model.ApprovedDecisionOutcome, output.Decision.Outcome)
	require.NotNil(t, output.InstallmentOffer)
	require.Equal(t, 4, output.InstallmentOffer.InstallmentCount)
	require.Len(t, output.InstallmentOffer.Installments, 4)
	require.Equal(t, installments[0].ID, output.InstallmentOffer.Installments[0].ID)
	require.Equal(t, model.OfferedInstallmentStatus, output.InstallmentOffer.Installments[0].Status)
	require.Equal(t, installments[0].TotalAmountMinor, output.InstallmentOffer.PayableNowAmountMinor)

	require.Equal(t, model.ApprovedFullFinancingDecisionReason, *auditEvent.Data.ReasonCode)
	require.Equal(t, []any{model.ApprovedFullFinancingDecisionReason}, auditEvent.Data.Metadata["reason_codes"])
	require.Equal(t, customerProfile.ID.String(), auditEvent.Data.Metadata["customer_id"])
	require.Equal(t, externalCustomerID.String(), auditEvent.Data.Metadata["identity_subject_id"])
	require.Equal(t, merchantProfile.ID.String(), auditEvent.Data.Metadata["merchant_profile_id"])
	require.Equal(t, merchantProfile.MerchantID.String(), auditEvent.Data.Metadata["merchant_id"])
	require.Equal(t, branch.ID.String(), auditEvent.Data.Metadata["merchant_branch_profile_id"])
	require.Equal(t, branch.BranchID.String(), auditEvent.Data.Metadata["branch_id"])
	require.Equal(t, string(model.SystemDecisionActorType), fmt.Sprint(auditEvent.Data.Metadata["decision_actor_type"]))
	require.EqualValues(t, 4, auditEvent.Data.Metadata["installment_count"])
	ids, ok := auditEvent.Data.Metadata["installment_ids"].([]any)
	require.True(t, ok)
	require.Len(t, ids, 4)
	require.Equal(t, installments[0].ID.String(), ids[0])
	require.Equal(t, installments[0].ID.String(), auditEvent.Data.Metadata["first_installment_id"])
	require.Equal(t, string(model.OfferedInstallmentStatus), fmt.Sprint(auditEvent.Data.Metadata["offered_installment_status"]))
	require.Len(t, auditEvent.Data.Changes, 4)
	require.Equal(t, "decision.outcome", auditEvent.Data.Changes[0].Field)
	require.Equal(t, "decision.approved_financed_amount_minor", auditEvent.Data.Changes[1].Field)
	require.Equal(t, "decision.upfront_amount_minor", auditEvent.Data.Changes[2].Field)
	require.Equal(t, "installments.status", auditEvent.Data.Changes[3].Field)
	require.Equal(t, string(model.OfferedInstallmentStatus), fmt.Sprint(auditEvent.Data.Changes[3].After))
}

func TestCreate_InstallmentCreateFailure_RollsBack(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCreateDeps(t, now)
	externalCustomerID, posGpsID, customerProfile, merchantProfile, branch, creditAccount := eligibleFixtures(now)
	decisionID := model.DecisionID(uuid.New())

	deps.expectConnTx(ctx)
	deps.expectTxBindings()
	deps.expectDefaultCheckoutPolicyAssignment(ctx)
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, testifymock.Anything).
		Return(model.Decision{}, cerr.ResourceNotFound(errors.New("missing"))).Once()
	deps.customerProfileTx.EXPECT().LockByExternalCustomerID(ctx, externalCustomerID).Return(customerProfile, nil).Once()
	deps.merchantBranchProfileTx.EXPECT().LockByPosGpsID(ctx, posGpsID).Return(branch, nil).Once()
	deps.merchantProfileTx.EXPECT().LockByMerchantID(ctx, branch.MerchantID).Return(merchantProfile, nil).Once()
	deps.creditAccountTx.EXPECT().GetByCustomerIDAndCurrency(
		ctx, customerProfile.ID, model.IRRCurrency,
	).Return(creditAccount, nil).Once()

	deps.decisionTx.EXPECT().Create(
		ctx,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		model.ApprovedDecisionOutcome,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Return(model.Decision{
		ID: decisionID, Outcome: model.ApprovedDecisionOutcome,
		ReasonCodes: []string{model.ApprovedFullFinancingDecisionReason},
		PolicyID:    "checkout_contract_approval", PolicyVersion: "v1",
	}, nil).Once()

	deps.allocationTx.EXPECT().Create(
		ctx,
		decisionID,
		"checkout-key",
		customerProfile.ID,
		merchantProfile.ID,
		&branch.ID,
		model.POSPurchaseChannel,
		posGpsID.UUID(),
		(*string)(nil),
		uint64(100_000),
		uint64(100_000),
		uint64(0),
		uint64(500_000),
		model.IRRCurrency,
		4,
		testifymock.Anything,
		now,
	).Return(model.CheckoutDecisionAllocation{
		DecisionID: decisionID, CustomerID: customerProfile.ID,
		MerchantProfileID: merchantProfile.ID, MerchantBranchProfileID: &branch.ID,
		PurchaseChannel:      model.POSPurchaseChannel,
		PurchaseSourceID:     posGpsID.UUID(),
		RequestedAmountMinor: 100_000, ApprovedFinancedAmountMinor: 100_000,
		Currency: model.IRRCurrency, InstallmentCount: 4,
	}, nil).Once()

	deps.installmentTx.EXPECT().Create(
		ctx,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Return(model.Installment{}, cerr.InternalError(errors.New("installment insert failed"))).Once()

	_, err := deps.newUseCase(t).Create(ctx, decisionuc.CreateInput{
		CheckoutKey:        "checkout-key",
		ExternalCustomerID: externalCustomerID,
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: posGpsID,
		},
		RequestedAmountMinor: 100_000,
		Currency:             model.IRRCurrency,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
}

func TestCreate_IdempotentReplay_ReturnsPersistedInstallments(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCreateDeps(t, now)
	externalCustomerID, posGpsID, customerProfile, merchantProfile, branch, _ := eligibleFixtures(now)
	decisionID := model.DecisionID(uuid.New())
	expiresAt := now.Add(15 * time.Minute)

	uc := deps.newUseCase(t)
	fingerprint, err := decisionuc.CheckoutFingerprintForTest(
		uc,
		"checkout-key",
		externalCustomerID,
		decisionuc.PurchaseContext{Channel: model.POSPurchaseChannel, PosGpsID: posGpsID},
		100_000,
		"IRR",
		"checkout_contract_approval",
		"v1",
	)
	require.NoError(t, err)

	deps.expectConnTx(ctx)
	deps.expectTxBindings()
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, "checkout-key").
		Return(model.Decision{
			ID: decisionID, Outcome: model.ApprovedDecisionOutcome,
			ReasonCodes:            []string{model.ApprovedFullFinancingDecisionReason},
			IdempotencyFingerprint: fingerprint,
			PolicyID:               "checkout_contract_approval", PolicyVersion: "v1",
		}, nil).Twice()

	allocation := model.CheckoutDecisionAllocation{
		DecisionID: decisionID, CustomerID: customerProfile.ID,
		MerchantProfileID: merchantProfile.ID, MerchantBranchProfileID: &branch.ID,
		PurchaseChannel:      model.POSPurchaseChannel,
		PurchaseSourceID:     posGpsID.UUID(),
		RequestedAmountMinor: 100_000, ApprovedFinancedAmountMinor: 100_000,
		AvailableCreditMinor: 500_000, Currency: model.IRRCurrency,
		InstallmentCount: 4, ExpiresAt: expiresAt,
	}
	deps.allocationTx.EXPECT().GetByDecisionID(ctx, decisionID).Return(allocation, nil).Once()

	installments := []model.Installment{{
		ID: model.InstallmentID(uuid.New()), DecisionID: decisionID, InstallmentOrder: 1,
		CustomerID: customerProfile.ID, MerchantProfileID: merchantProfile.ID,
		MerchantBranchProfileID: &branch.ID, Currency: model.IRRCurrency,
		PrincipalAmountMinor: 25_000, TotalAmountMinor: 25_000,
		Status: model.OfferedInstallmentStatus,
	}, {
		ID: model.InstallmentID(uuid.New()), DecisionID: decisionID, InstallmentOrder: 2,
		CustomerID: customerProfile.ID, MerchantProfileID: merchantProfile.ID,
		MerchantBranchProfileID: &branch.ID, Currency: model.IRRCurrency,
		PrincipalAmountMinor: 25_000, TotalAmountMinor: 25_000,
		Status: model.OfferedInstallmentStatus,
	}, {
		ID: model.InstallmentID(uuid.New()), DecisionID: decisionID, InstallmentOrder: 3,
		CustomerID: customerProfile.ID, MerchantProfileID: merchantProfile.ID,
		MerchantBranchProfileID: &branch.ID, Currency: model.IRRCurrency,
		PrincipalAmountMinor: 25_000, TotalAmountMinor: 25_000,
		Status: model.OfferedInstallmentStatus,
	}, {
		ID: model.InstallmentID(uuid.New()), DecisionID: decisionID, InstallmentOrder: 4,
		CustomerID: customerProfile.ID, MerchantProfileID: merchantProfile.ID,
		MerchantBranchProfileID: &branch.ID, Currency: model.IRRCurrency,
		PrincipalAmountMinor: 25_000, TotalAmountMinor: 25_000,
		Status: model.OfferedInstallmentStatus,
	}}
	deps.installmentTx.EXPECT().ListByDecisionID(ctx, decisionID).Return(installments, nil).Once()
	deps.installmentChargeTx.EXPECT().ListByInstallmentIDs(ctx, testifymock.Anything).
		Return([]model.InstallmentCharge{}, nil).Once()

	output, err := uc.Create(ctx, decisionuc.CreateInput{
		CheckoutKey:        "checkout-key",
		ExternalCustomerID: externalCustomerID,
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: posGpsID,
		},
		RequestedAmountMinor: 100_000,
		Currency:             model.IRRCurrency,
	})
	require.NoError(t, err)
	require.Equal(t, decisionuc.CreateResultAlreadyExists, output.Result)
	require.NotNil(t, output.InstallmentOffer)
	require.Equal(t, installments[0].ID, output.InstallmentOffer.Installments[0].ID)
	require.Equal(t, []string{model.ApprovedFullFinancingDecisionReason}, output.Decision.ReasonCodes)
}

func TestCreate_DeniedIdempotentReplay_ReturnsPersistedAvailableCredit(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCreateDeps(t, now)
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	posGpsID := model.PosGpsID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	expiresAt := now.Add(15 * time.Minute)

	uc := deps.newUseCase(t)
	fingerprint, err := decisionuc.CheckoutFingerprintForTest(
		uc,
		"checkout-key",
		externalCustomerID,
		decisionuc.PurchaseContext{Channel: model.POSPurchaseChannel, PosGpsID: posGpsID},
		100_000,
		"IRR",
		"checkout_contract_approval",
		"v1",
	)
	require.NoError(t, err)

	deps.expectConnTx(ctx)
	deps.expectTxBindings()
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, "checkout-key").Return(model.Decision{
		ID:                     decisionID,
		Outcome:                model.DeniedDecisionOutcome,
		ReasonCodes:            []string{model.RequestedAmountExceedsMerchantLimitDecisionReason},
		IdempotencyFingerprint: fingerprint,
		PolicyID:               "checkout_contract_approval",
		PolicyVersion:          "v1",
		ExpiresAt:              &expiresAt,
	}, nil).Twice()
	deps.allocationTx.EXPECT().GetByDecisionID(ctx, decisionID).Return(model.CheckoutDecisionAllocation{
		DecisionID:           decisionID,
		CheckoutKey:          "checkout-key",
		RequestedAmountMinor: 100_000,
		AvailableCreditMinor: 350_000,
		Currency:             model.IRRCurrency,
		InstallmentCount:     4,
		ExpiresAt:            expiresAt,
	}, nil).Once()

	output, err := uc.Create(ctx, decisionuc.CreateInput{
		CheckoutKey:        "checkout-key",
		ExternalCustomerID: externalCustomerID,
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: posGpsID,
		},
		RequestedAmountMinor: 100_000,
		Currency:             model.IRRCurrency,
	})
	require.NoError(t, err)
	require.Equal(t, decisionuc.CreateResultAlreadyExists, output.Result)
	require.Equal(t, uint64(350_000), output.AvailableCreditMinor)
	require.Equal(t, uint64(100_000), output.RequestedAmountMinor)
	require.Equal(t, model.IRRCurrency, output.Currency)
	require.Equal(t, &expiresAt, output.ExpiresAt)
	require.Nil(t, output.InstallmentOffer)
}

func TestCreate_ChangedBranch_IdempotencyConflict(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCreateDeps(t, now)
	externalCustomerID, posGpsID, _, _, _, _ := eligibleFixtures(now)
	otherBranchID := uuid.New()

	uc := deps.newUseCase(t)
	storedFingerprint, err := decisionuc.CheckoutFingerprintForTest(
		uc,
		"checkout-key",
		externalCustomerID,
		decisionuc.PurchaseContext{Channel: model.POSPurchaseChannel, PosGpsID: model.PosGpsID(otherBranchID)},
		100_000,
		"IRR",
		"checkout_contract_approval",
		"v1",
	)
	require.NoError(t, err)

	deps.expectConnTx(ctx)
	deps.expectTxBindings()
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, "checkout-key").
		Return(model.Decision{
			ID: model.DecisionID(uuid.New()), Outcome: model.ApprovedDecisionOutcome,
			IdempotencyFingerprint: storedFingerprint,
			PolicyID:               "checkout_contract_approval",
			PolicyVersion:          "v1",
		}, nil).Twice()

	_, err = uc.Create(ctx, decisionuc.CreateInput{
		CheckoutKey:        "checkout-key",
		ExternalCustomerID: externalCustomerID,
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: posGpsID,
		},
		RequestedAmountMinor: 100_000,
		Currency:             model.IRRCurrency,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeIdempotencyConflict))
}

func TestCreate_OriginationDisabled_FailsClosedBeforePersist(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCreateDeps(t, now)
	externalCustomerID, posGpsID, customerProfile, merchantProfile, branch, _ := eligibleFixtures(now)
	merchantProfile.Jurisdiction = model.Jurisdiction("TR")

	deps.expectConnTx(ctx)
	deps.expectTxBindings()
	deps.expectDefaultCheckoutPolicyAssignment(ctx)
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, testifymock.Anything).
		Return(model.Decision{}, cerr.ResourceNotFound(errors.New("missing"))).Once()
	deps.customerProfileTx.EXPECT().LockByExternalCustomerID(ctx, externalCustomerID).Return(customerProfile, nil).Once()
	deps.merchantBranchProfileTx.EXPECT().LockByPosGpsID(ctx, posGpsID).Return(branch, nil).Once()
	deps.merchantProfileTx.EXPECT().LockByMerchantID(ctx, branch.MerchantID).Return(merchantProfile, nil).Once()

	_, err := deps.newUseCase(t).Create(ctx, decisionuc.CreateInput{
		CheckoutKey:        "checkout-key",
		ExternalCustomerID: externalCustomerID,
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: posGpsID,
		},
		RequestedAmountMinor: 100_000,
		Currency:             model.IRRCurrency,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
	require.False(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestCreate_IdempotentReplay_AfterOriginationDisabled(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCreateDeps(t, now)
	blocked := mock.NewOriginationJurisdictions(t)
	deps.jurisdictions = blocked
	externalCustomerID, posGpsID, customerProfile, merchantProfile, branch, _ := eligibleFixtures(now)
	decisionID := model.DecisionID(uuid.New())
	expiresAt := now.Add(15 * time.Minute)

	uc := deps.newUseCase(t)
	fingerprint, err := decisionuc.CheckoutFingerprintForTest(
		uc,
		"checkout-key",
		externalCustomerID,
		decisionuc.PurchaseContext{Channel: model.POSPurchaseChannel, PosGpsID: posGpsID},
		100_000,
		"IRR",
		"checkout_contract_approval",
		"v1",
	)
	require.NoError(t, err)

	deps.expectConnTx(ctx)
	deps.expectTxBindings()
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, "checkout-key").
		Return(model.Decision{
			ID: decisionID, Outcome: model.ApprovedDecisionOutcome,
			ReasonCodes:            []string{model.ApprovedFullFinancingDecisionReason},
			IdempotencyFingerprint: fingerprint,
			PolicyID:               "checkout_contract_approval", PolicyVersion: "v1",
		}, nil).Twice()

	allocation := model.CheckoutDecisionAllocation{
		DecisionID: decisionID, CustomerID: customerProfile.ID,
		MerchantProfileID: merchantProfile.ID, MerchantBranchProfileID: &branch.ID,
		PurchaseChannel:      model.POSPurchaseChannel,
		PurchaseSourceID:     posGpsID.UUID(),
		RequestedAmountMinor: 100_000, ApprovedFinancedAmountMinor: 100_000,
		AvailableCreditMinor: 500_000, Currency: model.IRRCurrency,
		InstallmentCount: 4, ExpiresAt: expiresAt,
	}
	deps.allocationTx.EXPECT().GetByDecisionID(ctx, decisionID).Return(allocation, nil).Once()

	installments := []model.Installment{{
		ID: model.InstallmentID(uuid.New()), DecisionID: decisionID, InstallmentOrder: 1,
		CustomerID: customerProfile.ID, MerchantProfileID: merchantProfile.ID,
		MerchantBranchProfileID: &branch.ID, Currency: model.IRRCurrency,
		PrincipalAmountMinor: 25_000, TotalAmountMinor: 25_000,
		Status: model.OfferedInstallmentStatus,
	}, {
		ID: model.InstallmentID(uuid.New()), DecisionID: decisionID, InstallmentOrder: 2,
		CustomerID: customerProfile.ID, MerchantProfileID: merchantProfile.ID,
		MerchantBranchProfileID: &branch.ID, Currency: model.IRRCurrency,
		PrincipalAmountMinor: 25_000, TotalAmountMinor: 25_000,
		Status: model.OfferedInstallmentStatus,
	}, {
		ID: model.InstallmentID(uuid.New()), DecisionID: decisionID, InstallmentOrder: 3,
		CustomerID: customerProfile.ID, MerchantProfileID: merchantProfile.ID,
		MerchantBranchProfileID: &branch.ID, Currency: model.IRRCurrency,
		PrincipalAmountMinor: 25_000, TotalAmountMinor: 25_000,
		Status: model.OfferedInstallmentStatus,
	}, {
		ID: model.InstallmentID(uuid.New()), DecisionID: decisionID, InstallmentOrder: 4,
		CustomerID: customerProfile.ID, MerchantProfileID: merchantProfile.ID,
		MerchantBranchProfileID: &branch.ID, Currency: model.IRRCurrency,
		PrincipalAmountMinor: 25_000, TotalAmountMinor: 25_000,
		Status: model.OfferedInstallmentStatus,
	}}
	deps.installmentTx.EXPECT().ListByDecisionID(ctx, decisionID).Return(installments, nil).Once()
	deps.installmentChargeTx.EXPECT().ListByInstallmentIDs(ctx, testifymock.Anything).
		Return([]model.InstallmentCharge{}, nil).Once()

	output, err := uc.Create(ctx, decisionuc.CreateInput{
		CheckoutKey:        "checkout-key",
		ExternalCustomerID: externalCustomerID,
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: posGpsID,
		},
		RequestedAmountMinor: 100_000,
		Currency:             model.IRRCurrency,
	})
	require.NoError(t, err)
	require.Equal(t, decisionuc.CreateResultAlreadyExists, output.Result)
	require.NotNil(t, output.InstallmentOffer)
}

func mustTestJurisdictions(t *testing.T) *domaincapability.Jurisdictions {
	t.Helper()

	caps, err := domaincapability.NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction})
	require.NoError(t, err)

	return caps
}
