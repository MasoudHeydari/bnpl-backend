//go:build production

package command

const dbCmdLong = `Run database administration commands for schema initialization and
migration workflows.

Use init-prod for one-time production schema creation when bnpl is missing
(APP_ENVIRONMENT=production; refuses if the schema already exists), or migrate
to apply filesystem-backed incremental SQL migrations.`
