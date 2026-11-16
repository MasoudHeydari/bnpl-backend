package event

// Checkout decision and offer evaluation checkpoints.
var (
	DecisionEvaluationStarted = define(
		"decision.checkout.evaluation_started",
		"checkout decision evaluation started",
	)
	DecisionMerchantPolicyResolved = define(
		"decision.checkout.merchant_policy_resolved",
		"checkout merchant policy resolved",
	)
	DecisionAvailableCreditChecked = define(
		"decision.checkout.available_credit_checked",
		"available credit checked for checkout decision",
	)
	DecisionApproved = define(
		"decision.checkout.approved",
		"checkout decision approved",
	)
	DecisionPartiallyApproved = define(
		"decision.checkout.partially_approved",
		"checkout decision partially approved",
	)
	DecisionDenied = define(
		"decision.checkout.denied",
		"checkout decision denied",
	)
	DecisionPersisted = define(
		"decision.checkout.persisted",
		"checkout decision persisted",
	)
	DecisionReplayed = define(
		"decision.checkout.replayed",
		"existing checkout decision replayed",
	)
	DecisionOfferPrepared = define(
		"decision.offer.prepared",
		"checkout offer prepared from decision",
	)
	DecisionOfferRejected = define(
		"decision.offer.rejected",
		"checkout offer rejected",
	)
	DecisionOfferExpired = define(
		"decision.offer.expired",
		"checkout offer expired",
	)
	DecisionOfferInvalidated = define(
		"decision.offer.invalidated",
		"checkout offer invalidated",
	)
	DecisionPolicyValidationFailed = define(
		"decision.policy.validation_failed",
		"checkout decision policy validation failed",
	)
)
