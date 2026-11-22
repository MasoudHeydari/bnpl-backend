package event

// Checkout session and hosted-checkout flow checkpoints.
var (
	CheckoutSessionCreateRequested = define(
		"checkout.session.create_requested",
		"checkout session create requested",
	)
	CheckoutSessionInputRejected = define(
		"checkout.session.input_rejected",
		"checkout session input rejected",
	)
	CheckoutMerchantValidated = define(
		"checkout.merchant.validated",
		"checkout merchant and branch validated",
	)
	CheckoutSessionCreated = define(
		"checkout.session.created",
		"checkout session created",
	)
	CheckoutSessionReplayed = define(
		"checkout.session.replayed",
		"checkout session idempotent replay",
	)
	CheckoutSessionRetrieved = define(
		"checkout.session.retrieved",
		"checkout session retrieved",
	)
	CheckoutCustomerResolutionStarted = define(
		"checkout.customer.resolution_started",
		"checkout customer resolution started",
	)
	CheckoutCustomerResolved = define(
		"checkout.customer.resolved",
		"checkout customer resolved",
	)
	CheckoutCustomerResolutionFailed = define(
		"checkout.customer.resolution_failed",
		"checkout customer resolution failed",
	)
	CheckoutOfferPreparationStarted = define(
		"checkout.offer.preparation_started",
		"checkout offer preparation started",
	)
	CheckoutOfferPrepared = define(
		"checkout.offer.prepared",
		"checkout offer prepared",
	)
	CheckoutOfferPreparationFailed = define(
		"checkout.offer.preparation_failed",
		"checkout offer preparation failed",
	)
	CheckoutSessionStarted = define(
		"checkout.session.started",
		"checkout session started",
	)
	CheckoutSessionStartReplayed = define(
		"checkout.session.start_replayed",
		"checkout session start idempotent replay",
	)
	CheckoutConfirmRequested = define(
		"checkout.confirm.requested",
		"checkout confirmation requested",
	)
	CheckoutConfirmSagaStarted = define(
		"checkout.confirm.saga_started",
		"checkout confirmation saga started",
	)
	CheckoutConfirmSagaStepStarted = define(
		"checkout.confirm.saga_step_started",
		"checkout confirmation saga step started",
	)
	CheckoutConfirmSagaStepCompleted = define(
		"checkout.confirm.saga_step_completed",
		"checkout confirmation saga step completed",
	)
	CheckoutConfirmSagaFailed = define(
		"checkout.confirm.saga_failed",
		"checkout confirmation saga failed",
	)
	CheckoutConfirmSagaCompensating = define(
		"checkout.confirm.saga_compensating",
		"checkout confirmation saga compensation started",
	)
	CheckoutSessionCanceled = define(
		"checkout.session.canceled",
		"checkout session canceled",
	)
	CheckoutSessionExpired = define(
		"checkout.session.expired",
		"checkout session expired",
	)
	CheckoutTerminatedSafely = define(
		"checkout.session.terminated_safely",
		"checkout session terminated after verified absence of financial effect",
	)
	CheckoutSafeTerminationRejected = define(
		"checkout.session.safe_termination_rejected",
		"checkout safe termination rejected due to unresolved financial effect",
	)
	CheckoutHostedHandoffPrepared = define(
		"checkout.hosted.handoff_prepared",
		"hosted checkout handoff prepared",
	)
	CheckoutSessionExpirySweepCompleted = define(
		"checkout.session.expiry_sweep_completed",
		"checkout session expiry sweep completed",
	)
	CheckoutConfirmingRecoveryCompleted = define(
		"checkout.confirming.recovery_completed",
		"checkout confirming recovery completed",
	)
)
