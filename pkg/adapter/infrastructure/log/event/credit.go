package event

// Customer credit and limit assignment checkpoints.
var (
	CreditCustomerResolved = define(
		"credit.customer.resolved",
		"credit customer resolved",
	)
	CreditAssessmentStarted = define(
		"credit.assessment.started",
		"credit assessment started",
	)
	CreditAssessmentCompleted = define(
		"credit.assessment.completed",
		"credit assessment completed",
	)
	CreditLimitAssignmentAttempted = define(
		"credit.limit.assignment_attempted",
		"credit limit assignment attempted",
	)
	CreditLimitAssigned = define(
		"credit.limit.assigned",
		"credit limit assigned",
	)
	CreditLimitAlreadyAssigned = define(
		"credit.limit.already_assigned",
		"credit limit already assigned",
	)
	CreditLimitAssignmentDenied = define(
		"credit.limit.assignment_denied",
		"credit limit assignment denied",
	)
	CreditAvailabilityRetrieved = define(
		"credit.availability.retrieved",
		"credit availability retrieved",
	)
	CreditAccountUpdated = define(
		"credit.account.updated",
		"credit account updated",
	)
	CreditUsageRestricted = define(
		"credit.usage.restricted",
		"credit usage restriction encountered",
	)
	CreditExposureChanged = define(
		"credit.exposure.changed",
		"credit exposure changed",
	)
)
