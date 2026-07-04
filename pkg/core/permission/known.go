package permission

import "sync"

var (
	knownCodes     map[string]struct{}
	knownCodesOnce sync.Once
)

func loadKnownCodes() {
	all := All()
	knownCodes = make(map[string]struct{}, len(all))

	for _, perm := range all {
		knownCodes[perm.Code] = struct{}{}
	}
}

// Known reports whether code is a wire permission present in the BNPL catalog.
//
// Unknown strings must never be treated as granted permissions.
func Known(code string) bool {
	knownCodesOnce.Do(loadKnownCodes)

	_, ok := knownCodes[code]

	return ok
}

// FilterKnown returns only catalog wire codes from codes, preserving order and
// dropping unknowns. The result is never nil.
func FilterKnown(codes []string) []string {
	knownCodesOnce.Do(loadKnownCodes)

	out := make([]string, 0, len(codes))
	for _, code := range codes {
		if _, ok := knownCodes[code]; ok {
			out = append(out, code)
		}
	}

	return out
}
