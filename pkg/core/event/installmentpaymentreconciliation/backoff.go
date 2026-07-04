package installmentpaymentreconciliation

import (
	"fmt"
	"math"
	"time"
)

// RetryDelay computes an overflow-safe exponential reconciliation retry delay.
func RetryDelay(baseDelay, maxDelay time.Duration, attemptCount int) (time.Duration, error) {
	const op = "installmentpaymentreconciliation.RetryDelay"

	if baseDelay <= 0 {
		return 0, fmt.Errorf("%s: retry base delay must be positive", op)
	}

	if maxDelay < baseDelay {
		return 0, fmt.Errorf("%s: retry max delay must be greater than or equal to retry base delay", op)
	}

	if attemptCount <= 0 {
		attemptCount = 1
	}

	delay := baseDelay

	for i := 1; i < attemptCount; i++ {
		if delay >= maxDelay {
			return maxDelay, nil
		}

		halfMax := maxDelay / 2
		if delay > halfMax {
			return maxDelay, nil
		}

		next, err := safeDurationMultiply(delay, 2)
		if err != nil {
			return 0, err
		}

		delay = next
	}

	if delay > maxDelay {
		return maxDelay, nil
	}

	return delay, nil
}

func safeDurationMultiply(value time.Duration, factor int) (time.Duration, error) {
	const op = "installmentpaymentreconciliation.safeDurationMultiply"

	if factor <= 0 {
		return 0, fmt.Errorf("%s: duration multiplier must be positive", op)
	}

	if value == 0 {
		return 0, nil
	}

	if value > math.MaxInt64/time.Duration(factor) {
		return 0, fmt.Errorf("%s: duration multiplication overflow", op)
	}

	return value * time.Duration(factor), nil
}
