package customercredituc

// AssignInitialLimitInput carries the authenticated mobile used to resolve the
// Customer Reference Service customer before assigning an initial credit limit.
type AssignInitialLimitInput struct {
	// MobileE164 is the gateway-injected authenticated mobile
	// (x-auth-mobile-number ← JWT auth_mobile_number), canonical E.164.
	MobileE164 string
}
