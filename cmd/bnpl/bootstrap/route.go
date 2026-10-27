package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/healthrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/routes"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/shortlinkrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/swaggerrs"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
)

// RegisterRoutes registers the HTTP routes for App on the owned Gin engine.
//
// Metrics are not registered here. The Prometheus scrape endpoint runs on a
// dedicated operational HTTP server started from Run.
//
// e remains *gin.Engine here because bootstrap owns engine construction and
// global HTTP configuration; resource packages receive gin.IRouter instead.
func (a *App) RegisterRoutes(e *gin.Engine) error {
	const op = "bootstrap.App.RegisterRoutes"

	if err := a.ensureShortLinkAliases(context.Background()); err != nil {
		return fmt.Errorf("%s: ensure short link aliases: %w", op, err)
	}

	aliasCodes := []string{}
	if alias := strings.TrimSpace(a.Config.Usecases.ShortLink.RepaymentAlias); alias != "" {
		aliasCodes = append(aliasCodes, alias)
	}

	publicHost := strings.TrimSpace(a.Config.Usecases.ShortLink.PublicHost)
	shortlinkrs.Register(e, a.UseCases.ShortLink, shortlinkrs.RegisterOptions{
		Enabled:      a.Config.Usecases.ShortLink.Enabled,
		PublicHost:   publicHost,
		AliasCodes:   aliasCodes,
		DynamicCodes: a.Config.Usecases.ShortLink.Enabled && publicHost != "",
	})

	return routes.Register(
		e,
		a.UseCases.Health,
		a.UseCases.CheckoutSession,
		a.UseCases.Decision,
		a.UseCases.InstallmentRepayment,
		a.UseCases.Contract,
		a.UseCases.Merchant,
		a.UseCases.CustomerAccount,
		a.UseCases.CustomerCredit,
		a.UseCases.MerchantTerminal,
		routes.RegisterOptions{
			Health: healthrs.RegisterOptions{
				Enabled: a.Config.Usecases.Health.Enabled,
			},
			Swagger: swaggerrs.RegisterOptions{
				Enabled: a.Config.Gin.Swagger.Enabled,
			},
		},
	)
}

// ensureShortLinkAliases upserts configured static aliases when the shortener
// is enabled. Reminder SMS uses unique generated codes; aliases remain an
// optional separate shortener feature.
func (a *App) ensureShortLinkAliases(ctx context.Context) error {
	cfg := a.Config.Usecases.ShortLink
	if !cfg.Enabled || a.UseCases.ShortLink == nil {
		return nil
	}

	alias := strings.TrimSpace(cfg.RepaymentAlias)
	destination := strings.TrimSpace(cfg.RepaymentDestinationURL)
	if alias == "" || destination == "" {
		return nil
	}

	_, err := a.UseCases.ShortLink.EnsureAlias(ctx, shortlinkuc.EnsureAliasInput{
		Code:           alias,
		DestinationURL: destination,
	})
	return err
}
