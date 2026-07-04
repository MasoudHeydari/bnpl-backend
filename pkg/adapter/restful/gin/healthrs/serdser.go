package healthrs

// Response is the canonical JSON payload returned by health probe endpoints.
// Health probe states and diagnostics are machine-facing operational values and
// intentionally bypass user-facing localization.
type Response struct {
	Status string `json:"status" example:"UP" i18n:"canonical"`
	Error  string `json:"error,omitempty" i18n:"canonical"`
}
