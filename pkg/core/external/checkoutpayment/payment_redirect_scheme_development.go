//go:build development

package checkoutpayment

// isAllowedPaymentRedirectScheme reports whether scheme is acceptable for a
// Financial Core payment redirect in a development-capability binary.
func isAllowedPaymentRedirectScheme(scheme string) bool {
	return scheme == "https" || scheme == "http"
}

func paymentRedirectURLSchemeRequirement() string {
	return "payment url must be an absolute http or https url without user information"
}
