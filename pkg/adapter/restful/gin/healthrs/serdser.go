package healthrs

// Response is the JSON payload returned by health probe endpoints.
type Response struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}
