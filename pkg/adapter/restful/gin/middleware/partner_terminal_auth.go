package middleware

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantterminaluc"
)

// TerminalAuthenticator resolves a presented terminal id and API key to a
// merchant principal. *merchantterminaluc.UseCase implements it.
type TerminalAuthenticator interface {
	Authenticate(
		ctx context.Context,
		terminalID model.TerminalID,
		apiKey string,
	) (merchantterminal.MerchantPrincipal, error)
}

// PartnerTerminalAuth authenticates external partner requests under
// /api/v1/partner/** with X-Terminal-Id and X-API-Key only.
//
// On success it attaches:
//
//   - MerchantPrincipal (terminal-owned Merchant identity + permissions)
//   - permission.AuthContext with SubjectID = MerchantID and Permissions from
//     the terminal assignment. TenantID and ContextID are left zero here.
//
// Trusted Gateway TenantID/ContextID are NOT required by this middleware.
// Routes that snapshot Financial Core scope (Partner Create Checkout) must
// chain TrustedGatewayScope after this middleware.
//
// X-Auth-Subject-Id and X-Auth-Permissions are never trusted for Merchant
// identity or permissions on Partner routes — those remain terminal-owned.
//
// Merchant Admin Panel credential management (provision/rotate/retire/get)
// does NOT use this middleware; those routes use gateway AuthContext.Attach.
//
// Terminal credential headers must appear exactly once. Values are not trimmed
// or otherwise normalized: API keys are compared exactly, and terminal ids must
// be canonical decimal. Missing, duplicate, or malformed terminal credentials
// yield the same public unauthorized error as a wrong credential. The API key
// is never logged.
func PartnerTerminalAuth(uc *merchantterminaluc.UseCase) gin.HandlerFunc {
	return PartnerTerminalAuthWith(uc)
}

// PartnerTerminalAuthWith is PartnerTerminalAuth over any TerminalAuthenticator.
func PartnerTerminalAuthWith(authenticator TerminalAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "middleware.PartnerTerminalAuth"

		rawTerminalID, ok := singleHeaderValue(c, header.TerminalID)
		if !ok {
			serdser.SerErr(c, invalidTerminalCredentialsError(
				fmt.Errorf("%s: terminal id header is missing or duplicated", op),
			))
			c.Abort()

			return
		}

		apiKey, ok := singleHeaderValue(c, header.APIKey)
		if !ok {
			serdser.SerErr(c, invalidTerminalCredentialsError(
				fmt.Errorf("%s: api key header is missing or duplicated", op),
			))
			c.Abort()

			return
		}

		if rawTerminalID == "" || apiKey == "" {
			serdser.SerErr(c, invalidTerminalCredentialsError(
				fmt.Errorf("%s: terminal credentials are empty", op),
			))
			c.Abort()

			return
		}

		terminalID, err := model.ParseTerminalID(rawTerminalID)
		if err != nil {
			serdser.SerErr(c, invalidTerminalCredentialsError(
				fmt.Errorf("%s: terminal id is malformed: %w", op, err),
			))
			c.Abort()

			return
		}

		principal, err := authenticator.Authenticate(c.Request.Context(), terminalID, apiKey)
		if err != nil {
			serdser.SerErr(c, err)
			c.Abort()

			return
		}

		// Merchant identity + permissions come solely from the terminal.
		// TenantID/ContextID stay zero until TrustedGatewayScope (Create only).
		authCtx := permission.AuthContext{
			SubjectID:   model.SubjectID(principal.MerchantID),
			Permissions: append([]string(nil), principal.Permissions...),
		}

		ctx := merchantterminal.WithMerchantPrincipal(c.Request.Context(), principal)
		ctx = permission.WithAuthContext(ctx, authCtx)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// singleHeaderValue returns the sole value of header name. Missing headers and
// duplicates both report ok=false so callers can fail closed uniformly.
func singleHeaderValue(c *gin.Context, name string) (string, bool) {
	values := c.Request.Header.Values(name)
	if len(values) != 1 {
		return "", false
	}

	return values[0], true
}

// invalidTerminalCredentialsError matches the public error returned by
// merchantterminaluc.UseCase.Authenticate for rejected credentials.
func invalidTerminalCredentialsError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeUnauthorized,
		"error.invalid_terminal_credentials",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}
