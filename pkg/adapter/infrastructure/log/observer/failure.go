package observer

import (
	"context"
	"errors"
	"net"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/log"
)

// fcFailureFields returns bounded diagnostic fields for a failed Financial
// Core call: a stable error code when the failure carries one, plus the
// truncated error message.
func fcFailureFields(err error) []log.Field {
	if err == nil {
		return nil
	}

	fields := make([]log.Field, 0, 2)

	var applicationErr *cerr.Error
	if errors.As(err, &applicationErr) && applicationErr.Code != "" {
		fields = append(fields, field.ErrorCode(string(applicationErr.Code)))
	}

	return append(fields, field.ErrorMessage(err.Error()))
}

// isFCTimeout reports whether a Financial Core call outcome is unknown because
// the call timed out or was classified as ambiguous.
func isFCTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	var classified *extcheckoutpayment.ClassifiedFailure

	return errors.As(err, &classified) && classified.Class == extcheckoutpayment.FailureClassAmbiguous
}
