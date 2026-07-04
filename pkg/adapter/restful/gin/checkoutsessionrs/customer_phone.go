package checkoutsessionrs

// Customer phone REST handling is intentionally jurisdiction-agnostic.
//
// REST only enforces generic input safety (required + max length via binding).
// MerchantProfile.Jurisdiction selects phone validation and canonicalization in
// the checkout session use case. Do not add country prefixes, digit counts, or
// Iran/Turkey-specific regexes here.
