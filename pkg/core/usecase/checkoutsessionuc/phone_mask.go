package checkoutsessionuc

import "strings"

const (
	// iranE164CountryPrefix is replaced by the local trunk "0" for display.
	iranE164CountryPrefix = "+98"

	// maskedPhoneVisiblePrefix and maskedPhoneVisibleSuffix are the local-form
	// digits left readable around the mask.
	maskedPhoneVisiblePrefix = 4
	maskedPhoneVisibleSuffix = 4

	// phoneMask replaces the hidden middle digits.
	phoneMask = "***"
)

// MaskCustomerPhone renders a canonical E.164 Iranian mobile for display on the
// hosted checkout, for example "+989121234567" -> "0912***4567".
//
// Input that is not a canonical +98 mobile number is fully masked rather than
// partially revealed.
func MaskCustomerPhone(e164 string) string {
	if !strings.HasPrefix(e164, iranE164CountryPrefix) {
		return phoneMask
	}

	local := "0" + e164[len(iranE164CountryPrefix):]
	if len(local) <= maskedPhoneVisiblePrefix+maskedPhoneVisibleSuffix {
		return phoneMask
	}

	for i := 0; i < len(local); i++ {
		if local[i] < '0' || local[i] > '9' {
			return phoneMask
		}
	}

	return local[:maskedPhoneVisiblePrefix] + phoneMask + local[len(local)-maskedPhoneVisibleSuffix:]
}
