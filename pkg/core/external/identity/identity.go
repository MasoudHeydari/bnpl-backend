// Package identity defines the external Identity Service boundary used to
// resolve a customer subject from a merchant-asserted mobile number.
//
// For External Partner checkout, BNPL resolves the mobile during Partner Create
// (before OTP). OTP verification later proves phone possession and authorizes
// confirmation of the exact offer generation the customer saw. Identity remains
// the sole authority that maps a mobile to a subject (identity_subject_id).
//
// Identity subject status is evaluated for the current checkout resolution only
// and is never synchronized into BNPL CustomerProfile.status.
package identity
