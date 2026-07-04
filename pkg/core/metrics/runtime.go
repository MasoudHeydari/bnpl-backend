package metrics

// RuntimeMetrics is the complete metrics contract required by application
// bootstrap.
//
// Keeping this composition in Core avoids bootstrap type assertions against
// adapter-owned interfaces and guarantees at compile time that every configured
// metrics provider supports both business and outbound HTTP observations.
type RuntimeMetrics interface {
	// BusinessMetrics records domain and workflow observations.
	BusinessMetrics

	// OutboundHTTPMetrics records outbound HTTP client observations.
	OutboundHTTPMetrics
}
