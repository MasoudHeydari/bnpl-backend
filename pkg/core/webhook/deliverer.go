package webhook

import (
	"context"
	"net/url"
)

// Delivery is one signed HTTP POST to a merchant webhook endpoint.
type Delivery struct {
	// URL is the validated destination URL.
	URL *url.URL

	// Headers are request headers, including the signature headers.
	Headers map[string]string

	// Body is the exact byte sequence that was signed.
	Body []byte
}

// Deliverer performs a single webhook HTTP delivery attempt.
//
// Contract:
//   - nil means the endpoint answered with a 2xx status.
//   - Errors wrapped with messaging.NonRetryable are permanent (non-408/429 4xx,
//     other unexpected statuses such as 3xx, blocked addresses). The outbox
//     moves such rows to failed without consuming the remaining retry budget.
//   - Any other error is retryable (408, 429, 5xx, network failure, timeout).
//   - Redirects are never followed.
//   - Implementations must enforce the destination Policy again at connection
//     time against resolved addresses so DNS rebinding cannot bypass it.
//   - Error text must not contain the destination URL, response body, or secrets
//     because it is persisted in the outbox last_error column.
type Deliverer interface {
	// Deliver sends d and reports the classified outcome.
	Deliver(ctx context.Context, d Delivery) error
}
