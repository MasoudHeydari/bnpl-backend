// Package contractuc defines checkout contract creation and policy-driven activation.
// Some policies require an authoritative initial payment before activation, while
// others activate at contract creation and collect installments later.
//
// Contract activation and exposure consumption happen only after payment success.
// Checkout decision, installment offer creation, and payment_pending contract
// creation do not consume credit or reserve exposure.
package contractuc
