package header

const (
	// TerminalID carries the public external partner terminal identifier
	// (X-Terminal-Id).
	TerminalID = "X-Terminal-Id"

	// APIKey carries the external partner terminal API key (X-API-Key). It is a
	// secret and must never be logged.
	APIKey = "X-API-Key"
)
