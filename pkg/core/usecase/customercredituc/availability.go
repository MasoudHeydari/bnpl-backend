package customercredituc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// availableCreditForInquiry returns limit minus active financed exposure.
// Checkout decisions, offers, and payment_pending contracts do not reduce availability.
func availableCreditForInquiry(
	creditLimitAmountMinor uint64,
	activeExposureAmountMinor uint64,
) (available uint64, used uint64, err error) {
	used = activeExposureAmountMinor

	if creditLimitAmountMinor < used {
		return 0, used, nil
	}

	available, err = subtractAmountMinor(creditLimitAmountMinor, used)
	if err != nil {
		return 0, 0, err
	}

	return available, used, nil
}

func subtractAmountMinor(a, b uint64) (uint64, error) {
	const op = "customercredituc.subtractAmountMinor"

	result, err := model.SubtractAmountMinor(a, b)
	if err != nil {
		return 0, cerr.InternalError(fmt.Errorf("%s: amount subtraction: %w", op, err))
	}

	return result, nil
}
