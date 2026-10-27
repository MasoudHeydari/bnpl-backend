package shortlinkrp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const shortLinkColumns = `
	id,
	code,
	destination_url,
	expires_at,
	revoked_at,
	created_at,
	updated_at
`

const shortLinksCodeUniqueConstraint = "short_links_code_unique"

// GetByCode returns a short link by its public code.
func GetByCode[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	code string,
) (model.ShortLink, error) {
	const op = "shortlinkrp.GetByCode"

	code = strings.TrimSpace(code)
	if code == "" {
		return model.ShortLink{}, cerr.InternalError(fmt.Errorf("%s: code is required", op))
	}

	const query = `SELECT` + shortLinkColumns + `
FROM short_links
WHERE code = $1
`

	return postgres.QueryOne[model.ShortLink](
		ctx,
		q,
		"get short link by code",
		query,
		code,
	)
}

// GetByID returns a short link by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.ShortLinkID,
) (model.ShortLink, error) {
	const op = "shortlinkrp.GetByID"

	if id.IsZero() {
		return model.ShortLink{}, cerr.InternalError(fmt.Errorf("%s: id is required", op))
	}

	const query = `SELECT` + shortLinkColumns + `
FROM short_links
WHERE id = $1
`

	return postgres.QueryOne[model.ShortLink](
		ctx,
		q,
		"get short link by id",
		query,
		id,
	)
}

// Create stores a new short link.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.ShortLinkID,
	code string,
	destinationURL string,
	expiresAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.ShortLink, error) {
	const op = "shortlinkrp.Create"

	code = strings.TrimSpace(code)
	destinationURL = strings.TrimSpace(destinationURL)

	if id.IsZero() {
		return model.ShortLink{}, cerr.InternalError(fmt.Errorf("%s: id is required", op))
	}

	if code == "" {
		return model.ShortLink{}, cerr.InternalError(fmt.Errorf("%s: code is required", op))
	}

	if destinationURL == "" {
		return model.ShortLink{}, cerr.InternalError(fmt.Errorf("%s: destination url is required", op))
	}

	const query = `
INSERT INTO short_links (
	id,
	code,
	destination_url,
	expires_at,
	revoked_at,
	created_at,
	updated_at
) VALUES (
	$1, $2, $3, $4, NULL, $5, $6
)
RETURNING` + shortLinkColumns

	row, err := postgres.QueryOne[model.ShortLink](
		ctx,
		q,
		"create short link",
		query,
		id,
		code,
		destinationURL,
		expiresAt,
		createdAt.UTC(),
		updatedAt.UTC(),
	)
	if err != nil {
		if postgres.IsUniqueViolation(err, shortLinksCodeUniqueConstraint) {
			return model.ShortLink{}, cerr.Conflict(
				fmt.Errorf("%s: short link code already exists", op),
			)
		}

		return model.ShortLink{}, err
	}

	return row, nil
}

// EnsureAlias upserts a stable alias code.
func EnsureAlias[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.ShortLinkID,
	code string,
	destinationURL string,
	now time.Time,
) (model.ShortLink, error) {
	const op = "shortlinkrp.EnsureAlias"

	code = strings.TrimSpace(code)
	destinationURL = strings.TrimSpace(destinationURL)

	if id.IsZero() {
		return model.ShortLink{}, cerr.InternalError(fmt.Errorf("%s: id is required", op))
	}

	if code == "" {
		return model.ShortLink{}, cerr.InternalError(fmt.Errorf("%s: code is required", op))
	}

	if destinationURL == "" {
		return model.ShortLink{}, cerr.InternalError(fmt.Errorf("%s: destination url is required", op))
	}

	const query = `
INSERT INTO short_links (
	id,
	code,
	destination_url,
	expires_at,
	revoked_at,
	created_at,
	updated_at
) VALUES (
	$1, $2, $3, NULL, NULL, $4, $4
)
ON CONFLICT (code) DO UPDATE
SET
	destination_url = EXCLUDED.destination_url,
	revoked_at = NULL,
	updated_at = EXCLUDED.updated_at
WHERE short_links.destination_url IS DISTINCT FROM EXCLUDED.destination_url
   OR short_links.revoked_at IS NOT NULL
RETURNING` + shortLinkColumns

	row, err := postgres.QueryOne[model.ShortLink](
		ctx,
		q,
		"ensure short link alias",
		query,
		id,
		code,
		destinationURL,
		now.UTC(),
	)
	if err != nil {
		// When ON CONFLICT DO UPDATE matches but WHERE filters all rows,
		// RETURNING is empty. Fall back to a plain read of the existing alias.
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return GetByCode(ctx, q, code)
		}

		return model.ShortLink{}, err
	}

	return row, nil
}

// Revoke marks a short link revoked.
func Revoke[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	code string,
	revokedAt time.Time,
) (model.ShortLink, error) {
	const op = "shortlinkrp.Revoke"

	code = strings.TrimSpace(code)
	if code == "" {
		return model.ShortLink{}, cerr.InternalError(fmt.Errorf("%s: code is required", op))
	}

	const query = `
UPDATE short_links
SET
	revoked_at = COALESCE(revoked_at, $2),
	updated_at = $2
WHERE code = $1
RETURNING` + shortLinkColumns

	return postgres.QueryOne[model.ShortLink](
		ctx,
		q,
		"revoke short link",
		query,
		code,
		revokedAt.UTC(),
	)
}
