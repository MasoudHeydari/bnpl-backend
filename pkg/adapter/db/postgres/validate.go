package postgres

import (
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// RequiredTime reports whether t was supplied by the caller.
//
// Repository adapters must not generate timestamps. Callers at the use-case
// layer are responsible for providing business time values.
func RequiredTime(field string, t time.Time) error {
	if t.IsZero() {
		return cerr.InvalidArgument(fmt.Errorf("%s is required", field))
	}

	return nil
}

// RequiredTimes validates multiple required timestamps.
func RequiredTimes(fields map[string]time.Time) error {
	for field, value := range fields {
		if err := RequiredTime(field, value); err != nil {
			return err
		}
	}

	return nil
}

func NullIfEmpty(value string) any {
	if value == "" {
		return nil
	}

	return value
}
