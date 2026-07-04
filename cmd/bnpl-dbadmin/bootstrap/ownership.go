package bootstrap

import (
	"context"
	"fmt"
	"io"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/schemarp"
	"github.com/parstis/bnpl-backend/pkg/adapter/dbadminconfig"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Ownership inventories first; apply is explicit and uses the existing migration
// advisory lock, a transaction and managed-schema-only ownership statements.
func Ownership(w io.Writer, apply bool) error {
	return configured(func(ctx context.Context, c *dbadminconfig.Config, l corelog.Logger) error {
		p, e := c.ConnectionPool(ctx, repo.AdminRole, l)
		if e != nil {
			return e
		}
		defer p.Close()
		return p.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			q := postgres.MustConn(conn)
			if _, e = q.Exec(ctx, `SET lock_timeout='30s'`); e != nil {
				return e
			}

			if e = migrationsrp.AcquireLock(ctx, q); e != nil {
				return e
			}
			defer migrationsrp.ReleaseLock(context.WithoutCancel(ctx), q)
			transaction := conn.Tx
			if !apply {
				transaction = conn.ReadSnapshot
			}
			return transaction(ctx, func(ctx context.Context, tx repo.Tx) error {
				tq := postgres.MustTx(tx)
				inventory, e := schemarp.SecurityInventory(ctx, tq)
				if e != nil {
					return e
				}
				for _, line := range inventory {
					fmt.Fprintln(w, line)
				}
				plan, e := schemarp.OwnershipPlan(ctx, tq)
				if e != nil {
					return e
				}
				for _, v := range plan {
					fmt.Fprintf(w, "%s owner=%s -> %s\n", v.Object, v.Owner, repo.OwnerRole)
				}

				if !apply {
					return nil
				}
				for _, r := range []repo.Role{repo.OwnerRole, repo.MigratorRole, repo.NormalRole} {
					password := ""
					if r != repo.OwnerRole {
						password, e = c.GetRolePassword(r)
						if e != nil {
							return e
						}
					}

					if e = schemarp.CreateRoleIfNotExists(ctx, tq, r, password); e != nil {
						return e
					}
				}

				if e = schemarp.GrantPrivileges(ctx, tq, "bnpl", repo.MigratorRole); e != nil {
					return e
				}

				if e = schemarp.ApplyOwnership(ctx, tq, plan); e != nil {
					return e
				}
				for _, r := range []repo.Role{repo.MigratorRole, repo.NormalRole} {
					if e = schemarp.SetSearchPath(ctx, tq, "bnpl", r); e != nil {
						return e
					}
				}
				return nil
			})
		})
	})
}
