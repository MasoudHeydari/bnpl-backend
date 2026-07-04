package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/parstis/bnpl-backend/pkg/core/webhook"
)

// SignedPayload builds the canonical byte string covered by the signature:
//
//	v1 LF <event-id> LF <unix-timestamp> LF <body>
//
// Newlines are unambiguous separators because event ids never contain them and
// the timestamp is decimal.
func SignedPayload(eventID string, timestamp int64, body []byte) []byte {
	ts := strconv.FormatInt(timestamp, 10)

	payload := make([]byte, 0, len(webhook.SignatureVersion)+len(eventID)+len(ts)+len(body)+3)
	payload = append(payload, webhook.SignatureVersion...)
	payload = append(payload, '\n')
	payload = append(payload, eventID...)
	payload = append(payload, '\n')
	payload = append(payload, ts...)
	payload = append(payload, '\n')
	payload = append(payload, body...)

	return payload
}

// Sign returns the X-BNPL-Webhook-Signature header value for the request.
func Sign(secret []byte, eventID string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(SignedPayload(eventID, timestamp, body))

	return webhook.SignatureVersion + "=" + hex.EncodeToString(mac.Sum(nil))
}
