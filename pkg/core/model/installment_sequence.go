package model

// NextPayableInSequence returns the earliest unpaid schedule installment by
// installment_order (scheduled, due, or overdue).
//
// Installments must be paid in order: a later installment is not payable until
// every earlier unpaid schedule installment has been paid.
func NextPayableInSequence(installments []Installment) (Installment, bool) {
	var next Installment
	found := false

	for _, installment := range installments {
		if !installment.Status.IsActive() {
			continue
		}

		if !found || installment.InstallmentOrder < next.InstallmentOrder {
			next = installment
			found = true
		}
	}

	return next, found
}

// IsNextPayableInSequence reports whether target is the next installment that
// may be paid under sequential repayment rules.
func IsNextPayableInSequence(target Installment, installments []Installment) bool {
	next, ok := NextPayableInSequence(installments)
	return ok && next.ID == target.ID
}
