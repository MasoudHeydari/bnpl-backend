//go:build production

package checkoutpayment

// isAllowedPaymentRedirectScheme reports whether scheme is acceptable for a
// Financial Core payment redirect in a production-capability binary.
//
// HTTP is intentionally unavailable in this build: runtime configuration cannot
// re-enable it.
func isAllowedPaymentRedirectScheme(scheme string) bool {
	return scheme == "https"
}

func paymentRedirectURLSchemeRequirement() string {
	return "payment url must be an absolute https url without user information"
}
