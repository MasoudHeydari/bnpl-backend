package paymentapplication

import "github.com/parstis/bnpl-backend/pkg/core/model"

// ReconciliationReason identifies why a Financial Core payment could not be applied automatically.
type ReconciliationReason = model.PaymentReconciliationReason

// UnsupportedPaymentResultStatusFailureCode is persisted on permanently failed inbox rows
// when a trusted payment-result envelope carries an unsupported status value.
const UnsupportedPaymentResultStatusFailureCode = "unsupported_payment_result_status"

// EventIdentityPayloadMismatchFailureCode is persisted when a replayed event id carries
// a different payload fingerprint than the original durable inbox record.
const EventIdentityPayloadMismatchFailureCode = "event_identity_payload_mismatch"
