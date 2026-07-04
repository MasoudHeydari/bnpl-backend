//go:build development

package command

const dbCmdLong = `Run database administration commands for schema initialization and
migration workflows.

Use init-dev to destructively recreate a development schema
(APP_ENVIRONMENT=development), or migrate to apply filesystem-backed
incremental SQL migrations.`
