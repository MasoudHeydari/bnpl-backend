package checkoutpayment

import (
	"fmt"
	"strings"
)

// CheckoutPayeeFromPrepare maps PrepareCollectionObligation payment_recipient_*
// fields onto CreateCheckoutPayment payee parties.
//
// Financial Core owns the cash recipient. BNPL must not invent platform (or any
// other type) when payment_recipient_type is missing from an older server.
func CheckoutPayeeFromPrepare(result PrepareCollectionObligationResult) (payeeType string, payeeID string, err error) {
	const op = "checkoutpayment.CheckoutPayeeFromPrepare"

	payeeType = strings.ToLower(strings.TrimSpace(result.PaymentRecipientType))
	if payeeType == "" {
		return "", "", fmt.Errorf(
			"%s: payment_recipient_type is required from PrepareCollectionObligation",
			op,
		)
	}

	payeeID = strings.TrimSpace(result.PaymentRecipientID)

	switch payeeType {
	case PartyPlatform:
		if payeeID != "" {
			return "", "", fmt.Errorf(
				"%s: platform payment_recipient_id must be empty (got %q)",
				op,
				payeeID,
			)
		}

	case PartyCustomer, "subject":
		return "", "", fmt.Errorf("%s: unsupported payment_recipient_type %q", op, payeeType)

	default:
		if payeeID == "" {
			return "", "", fmt.Errorf(
				"%s: payment_recipient_id is required for payment_recipient_type %q",
				op,
				payeeType,
			)
		}
	}

	return payeeType, payeeID, nil
}
