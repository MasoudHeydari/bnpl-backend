package paymentresult

import "strings"

// NormalizePaymentStatus trims the raw Financial Core payment status once.
func NormalizePaymentStatus(raw string) string {
	return strings.TrimSpace(raw)
}
