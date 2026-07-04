package webhook

// Request header names sent with every webhook delivery.
const (
	// HeaderID carries the stable event id. It is identical across retries so
	// receivers can deduplicate.
	HeaderID = "X-BNPL-Webhook-Id"

	// HeaderTimestamp carries the Unix timestamp (seconds) of this attempt. It is
	// covered by the signature so receivers can bound replay windows.
	HeaderTimestamp = "X-BNPL-Webhook-Timestamp"

	// HeaderSignature carries "v1=<hex(HMAC-SHA256)>".
	HeaderSignature = "X-BNPL-Webhook-Signature"
)

// SignatureVersion prefixes the signature header value and the signed string.
const SignatureVersion = "v1"
