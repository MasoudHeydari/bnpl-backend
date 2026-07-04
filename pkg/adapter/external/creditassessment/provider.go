package creditassessment

// Provider identifies an external credit assessment implementation.
type Provider string

const (
	// NoopProvider returns an empty signal set without calling an external service.
	NoopProvider Provider = "noop"

	// CreditBureauProvider is the future real external credit assessment integration.
	CreditBureauProvider Provider = "creditbureau"
)
