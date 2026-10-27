package bootstrap

import (
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/healthrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/routes"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/shortlinkrs"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/swaggerrs"
)

// RegisterRoutes registers the HTTP routes for App on the owned Gin engine.
//
// Metrics are not registered here. The Prometheus scrape endpoint runs on a
// dedicated operational HTTP server started from Run.
//
// e remains *gin.Engine here because bootstrap owns engine construction and
// global HTTP configuration; resource packages receive gin.IRouter instead.
func (a *App) RegisterRoutes(e *gin.Engine) error {
	aliasCodes := []string{}
	if alias := a.Config.Usecases.ShortLink.RepaymentAlias; alias != "" {
		aliasCodes = append(aliasCodes, alias)
	}

	shortlinkrs.Register(e, a.UseCases.ShortLink, shortlinkrs.RegisterOptions{
		Enabled:    a.Config.Usecases.ShortLink.Enabled,
		AliasCodes: aliasCodes,
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
