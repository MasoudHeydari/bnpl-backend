package shortlinkuc

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/metrics"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/shortlinkdestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	defaultCodeLength    = 8
	maxCodeCreateRetries = 8
	codeAlphabet         = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// UseCase orchestrates opaque short-link creation and resolution.
type UseCase struct {
	pool              repo.Pool
	shortLink         repo.ShortLink
	destinationPolicy shortlinkdestination.Policy
	clock             clock.Clock
	idGenerator       identifier.Generator
	businessMetrics   metrics.BusinessMetrics
	codeLength        int
}

// New constructs a short-link use case.
func New(
	pool repo.Pool,
	shortLink repo.ShortLink,
	destinationPolicy shortlinkdestination.Policy,
	clk clock.Clock,
	idGenerator identifier.Generator,
	businessMetrics metrics.BusinessMetrics,
) (*UseCase, error) {
	const op = "shortlinkuc.New"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if shortLink == nil {
		return nil, fmt.Errorf("%s: short link repository is required", op)
	}

	if destinationPolicy == nil {
		return nil, fmt.Errorf("%s: destination policy is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: id generator is required", op)
	}

	if businessMetrics == nil {
		return nil, fmt.Errorf("%s: business metrics is required", op)
	}

	return &UseCase{
		pool:              pool,
		shortLink:         shortLink,
		destinationPolicy: destinationPolicy,
		clock:             clk,
		idGenerator:       idGenerator,
		businessMetrics:   businessMetrics,
		codeLength:        defaultCodeLength,
	}, nil
}

// CreateInput carries short-link creation inputs.
type CreateInput struct {
	// DestinationURL is the HTTPS destination browsers are redirected to.
	DestinationURL string

	// ExpiresAt is an optional absolute expiry instant.
	ExpiresAt *time.Time
}

// CreateOutput reports a newly created short link.
type CreateOutput struct {
	// ShortLink is the persisted short link.
	ShortLink model.ShortLink
}

// Create stores a new short link with a cryptographically unpredictable code.
func (uc *UseCase) Create(ctx context.Context, input CreateInput) (CreateOutput, error) {
	var created CreateOutput

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			out, createErr := uc.CreateOnTx(ctx, tx, input)
			if createErr != nil {
				return createErr
			}

			created = out
			return nil
		})
	})
	if err != nil {
		return CreateOutput{}, err
	}

	uc.businessMetrics.ShortLinkCreated(ctx, "generated")

	return created, nil
}

// CreateOnTx stores a new short link inside an existing transaction so callers
// (for example repayment reminder dispatch) can compose Short Link, Reminder,
// and Outbox writes atomically.
func (uc *UseCase) CreateOnTx(
	ctx context.Context,
	tx repo.Tx,
	input CreateInput,
) (CreateOutput, error) {
	const op = "shortlinkuc.UseCase.CreateOnTx"

	if tx == nil {
		return CreateOutput{}, fmt.Errorf("%s: tx is required", op)
	}

	destination := strings.TrimSpace(input.DestinationURL)
	if err := uc.destinationPolicy.ValidateDestination(destination); err != nil {
		return CreateOutput{}, cerr.InvalidArgument(fmt.Errorf("%s: %w", op, err))
	}

	now := uc.clock.Now().UTC()
	if input.ExpiresAt != nil && !input.ExpiresAt.After(now) {
		return CreateOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: expires_at must be in the future", op),
		)
	}

	shortLinkQ := uc.shortLink.Tx(tx)

	for attempt := 0; attempt < maxCodeCreateRetries; attempt++ {
		code, genErr := generateCode(uc.codeLength)
		if genErr != nil {
			return CreateOutput{}, fmt.Errorf("%s: generate code: %w", op, genErr)
		}

		id, idErr := parseShortLinkID(uc.idGenerator.Generate())
		if idErr != nil {
			return CreateOutput{}, fmt.Errorf("%s: parse short link id: %w", op, idErr)
		}

		row, createErr := shortLinkQ.Create(
			ctx,
			id,
			code,
			destination,
			input.ExpiresAt,
			now,
			now,
		)
		if createErr != nil {
			if cerr.IsCode(createErr, cerr.CodeConflict) {
				uc.businessMetrics.ShortLinkCodeCollision(ctx)
				continue
			}

			return CreateOutput{}, createErr
		}

		return CreateOutput{ShortLink: row}, nil
	}

	return CreateOutput{}, fmt.Errorf("%s: exhausted short link code retries", op)
}

// EnsureAliasInput carries static alias upsert inputs.
type EnsureAliasInput struct {
	// Code is the stable public alias (for example "pay").
	Code string

	// DestinationURL is the HTTPS destination browsers are redirected to.
	DestinationURL string
}

// EnsureAliasOutput reports the ensured alias short link.
type EnsureAliasOutput struct {
	// ShortLink is the persisted alias short link.
	ShortLink model.ShortLink
}

// EnsureAlias upserts a stable alias used by trusted internal flows.
func (uc *UseCase) EnsureAlias(
	ctx context.Context,
	input EnsureAliasInput,
) (EnsureAliasOutput, error) {
	const op = "shortlinkuc.UseCase.EnsureAlias"

	code := strings.TrimSpace(input.Code)
	if code == "" {
		return EnsureAliasOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: code is required", op),
		)
	}

	destination := strings.TrimSpace(input.DestinationURL)
	if err := uc.destinationPolicy.ValidateDestination(destination); err != nil {
		return EnsureAliasOutput{}, cerr.InvalidArgument(fmt.Errorf("%s: %w", op, err))
	}

	now := uc.clock.Now().UTC()

	var ensured model.ShortLink

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			shortLinkQ := uc.shortLink.Tx(tx)

			id, idErr := parseShortLinkID(uc.idGenerator.Generate())
			if idErr != nil {
				return fmt.Errorf("%s: parse short link id: %w", op, idErr)
			}

			row, ensureErr := shortLinkQ.EnsureAlias(ctx, id, code, destination, now)
			if ensureErr != nil {
				return ensureErr
			}

			ensured = row
			return nil
		})
	})
	if err != nil {
		return EnsureAliasOutput{}, err
	}

	uc.businessMetrics.ShortLinkCreated(ctx, "alias")

	return EnsureAliasOutput{ShortLink: ensured}, nil
}

// ResolveOutput carries a successful short-link resolution.
type ResolveOutput struct {
	// DestinationURL is the HTTPS location to redirect to.
	DestinationURL string

	// ShortLink is the resolved short link row.
	ShortLink model.ShortLink
}

// Resolve looks up a public code and returns its active destination.
func (uc *UseCase) Resolve(ctx context.Context, code string) (ResolveOutput, error) {
	const op = "shortlinkuc.UseCase.Resolve"

	started := uc.clock.Now().UTC()
	code = strings.TrimSpace(code)
	if code == "" {
		uc.businessMetrics.ShortLinkResolved(ctx, "invalid")
		return ResolveOutput{}, cerr.InvalidArgument(fmt.Errorf("%s: code is required", op))
	}

	var link model.ShortLink

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		shortLinkQ := uc.shortLink.Conn(conn)
		row, getErr := shortLinkQ.GetByCode(ctx, code)
		if getErr != nil {
			return getErr
		}

		link = row
		return nil
	})
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			uc.businessMetrics.ShortLinkResolved(ctx, "unknown")
			uc.observeResolveLatency(ctx, started)
			return ResolveOutput{}, err
		}

		uc.businessMetrics.ShortLinkResolved(ctx, "error")
		uc.observeResolveLatency(ctx, started)
		return ResolveOutput{}, err
	}

	now := uc.clock.Now().UTC()
	if link.IsRevoked() {
		uc.businessMetrics.ShortLinkResolved(ctx, "revoked")
		uc.observeResolveLatency(ctx, started)
		return ResolveOutput{}, cerr.ResourceNotFound(
			fmt.Errorf("%s: short link revoked", op),
		)
	}

	if link.IsExpired(now) {
		uc.businessMetrics.ShortLinkResolved(ctx, "expired")
		uc.observeResolveLatency(ctx, started)
		return ResolveOutput{}, cerr.ResourceNotFound(
			fmt.Errorf("%s: short link expired", op),
		)
	}

	uc.businessMetrics.ShortLinkResolved(ctx, "ok")
	uc.observeResolveLatency(ctx, started)

	return ResolveOutput{
		DestinationURL: link.DestinationURL,
		ShortLink:      link,
	}, nil
}

func (uc *UseCase) observeResolveLatency(ctx context.Context, started time.Time) {
	uc.businessMetrics.ShortLinkResolveLatency(ctx, uc.clock.Now().UTC().Sub(started))
}

func parseShortLinkID(raw string) (model.ShortLinkID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return model.ShortLinkID{}, err
	}

	return model.ShortLinkID(id), nil
}

func generateCode(length int) (string, error) {
	const op = "shortlinkuc.generateCode"

	if length <= 0 {
		return "", fmt.Errorf("%s: length must be positive", op)
	}

	alphabetSize := big.NewInt(int64(len(codeAlphabet)))
	buf := make([]byte, length)

	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, alphabetSize)
		if err != nil {
			return "", fmt.Errorf("%s: read random: %w", op, err)
		}

		buf[i] = codeAlphabet[n.Int64()]
	}

	return string(buf), nil
}
