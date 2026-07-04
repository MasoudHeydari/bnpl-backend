// Package phonenumber defines the Core port for jurisdiction-aware customer
// phone validation and canonicalization.
//
// This package lives under the canonicalizer namespace. That namespace groups
// concept-specific value canonicalizers; it is not a universal Canonicalizer
// interface. Future concepts (for example nationalid or iban) should define
// their own contracts under pkg/core/canonicalizer/<concept>.
//
// Implementations are local adapters. They must not perform network I/O.
// Phone ownership is not checked here: Identity.ResolveSubjectByMobile resolves
// the canonical phone to a subject, and Hosted OTP later proves possession.
//
// The jurisdiction argument comes from MerchantProfile.Jurisdiction. Country is
// never inferred by trying validators based on the phone prefix.
//
// Origination admission (whether a jurisdiction may create new BNPL business)
// is owned by pkg/core/domaincapability, not by this package.
package phonenumber
