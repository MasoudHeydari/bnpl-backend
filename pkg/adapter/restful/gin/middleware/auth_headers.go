package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// requireTrustedAuthUUIDHeader reads a Gateway-injected auth UUID header.
//
// X-Auth-* headers are trusted only because the Gateway strips client-supplied
// values and injects values derived from the authenticated token. BNPL never
// parses JWTs itself. The header must appear exactly once with a canonical
// non-zero UUID; missing, duplicate, empty, or malformed values are Unauthorized.
func requireTrustedAuthUUIDHeader(c *gin.Context, name string) (uuid.UUID, error) {
	raw, ok := singleHeaderValue(c, name)
	if !ok || raw == "" {
		return uuid.Nil, authHeaderRequiredError(name)
	}

	parsed, err := uuid.Parse(raw)
	if err != nil || parsed == uuid.Nil {
		return uuid.Nil, authHeaderInvalidError(name)
	}

	return parsed, nil
}

// authHeaderRequiredError is returned when a trusted gateway auth header is
// missing or duplicated. MessageKey stays the stable Unauthorized default;
// Details.Fields names the concrete header for clients.
func authHeaderRequiredError(headerName string) error {
	return cerr.NewPublic(
		cerr.CodeUnauthorized,
		cerr.DefaultMessageKey(cerr.CodeUnauthorized),
		fmt.Errorf("trusted auth header %q is missing or duplicated", headerName),
		cerr.Details{
			Fields: []string{headerName},
			Violations: []cerr.Violation{
				{
					Code:          "REQUIRED",
					MessageKey:    "validation.header_required",
					MessageParams: cerr.MessageParams{"header": headerName},
					Fields:        []string{headerName},
				},
			},
		},
	)
}

// authHeaderInvalidError is returned when a trusted gateway auth header is
// present but not a usable non-zero UUID.
func authHeaderInvalidError(headerName string) error {
	return cerr.NewPublic(
		cerr.CodeUnauthorized,
		cerr.DefaultMessageKey(cerr.CodeUnauthorized),
		fmt.Errorf("trusted auth header %q is invalid", headerName),
		cerr.Details{
			Fields: []string{headerName},
			Violations: []cerr.Violation{
				{
					Code:          "INVALID",
					MessageKey:    "validation.header_invalid",
					MessageParams: cerr.MessageParams{"header": headerName},
					Fields:        []string{headerName},
				},
			},
		},
	)
}
