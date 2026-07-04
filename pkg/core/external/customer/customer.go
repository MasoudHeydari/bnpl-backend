// Package customer defines the external Customer Reference Service boundary
// used to resolve a customer from a merchant-asserted mobile number.
//
// For Partner checkout, BNPL resolves the mobile during Create (on_create) or
// Start (on_start) before OTP. OTP verification later proves phone possession
// and authorizes confirmation of the exact offer generation the customer saw.
// The Customer Reference Service is the sole authority that maps a mobile to an
// external customer_id stored in customer_profiles.external_customer_id.
//
// Customer status is evaluated for the current checkout resolution only and is
// never synchronized into BNPL CustomerProfile.status.
//
// Do not conflate the CRS customer_id with the BNPL-local customer_profiles.id
// (model.CustomerID).
package customer
