// Package domaincapability models deployment-enabled business admission
// capabilities, such as which jurisdictions may originate new BNPL business.
//
// model.SupportedJurisdictions() declares what this BNPL binary can support.
// domain.supported_jurisdictions (configuration) selects the immutable subset
// enabled for new-business/origination in a deployment.
//
// Business consumers depend on OriginationJurisdictions. Composition roots
// construct a single *Jurisdictions instance and inject it into Decision,
// Checkout, Contract, and Phone Registry wiring.
//
// This package does not contain generic domain services such as value
// canonicalizers. Phone parsing lives under pkg/core/canonicalizer/phonenumber;
// the phone registry consumes Jurisdictions but is not itself a capability.
package domaincapability
