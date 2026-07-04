// Package checkout defines the unified checkout-decision and
// installment-offer policy family: customer/merchant/branch eligibility,
// financing and installment allocation, and installment schedule
// construction. A single policy version governs all three stages so that a
// checkout decision persisted under a given PolicyID/PolicyVersion always
// replays with the exact math that produced its persisted allocation,
// regardless of which version later becomes active for new decisions.
package checkout
