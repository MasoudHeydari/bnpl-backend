package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CheckoutSessionCreditKindTxQueryer mutates the Partner-selected credit kind
// on an open checkout. Prefer this focused surface over ad-hoc session updates.
//
// Concrete adapters typically implement this alongside CheckoutSessionTxQueryer.
type CheckoutSessionCreditKindTxQueryer interface {
	// ReplaceCreditKindAndInvalidateEvaluation persists a first selection or a
	// genuine replacement under optimistic concurrency.
	//
	// First selection (credit_kind currently unset): stores kind without bumping
	// evaluation_generation.
	//
	// Replacement (credit_kind currently set to a different value): stores the
	// new kind, clears decision_id, and increments evaluation_generation exactly
	// once. Prior Decisions remain immutable history under their generation keys.
	//
	// The row must still be open, at expectedGeneration, linked to
	// expectedDecisionID (nil means no Decision linked), have contract_id NULL,
	// and have confirmation_attempt_id NULL. A lost race or disallowed lifecycle
	// state is reported as cerr.Conflict.
	//
	// Callers must short-circuit same-kind idempotent replays before invoking
	// this method so generation is never advanced for a no-op.
	ReplaceCreditKindAndInvalidateEvaluation(
		context.Context,
		model.CheckoutSessionID,
		model.CreditKind,
		int,
		*model.DecisionID,
		time.Time,
	) (model.CheckoutSession, error)
}
