package querylog

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRender_ReplacesPositionalArguments(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	at := time.Date(
		2026,
		time.September,
		3,
		12,
		30,
		0,
		123,
		time.UTC,
	)

	rendered, err := Render(
		"SELECT * FROM payments WHERE id = $1 AND status = $2 AND created_at <= $3 AND retry = $4",
		[]any{id, "succeeded", at, true},
	)
	require.NoError(t, err)
	require.Equal(
		t,
		"SELECT * FROM payments WHERE id = '550e8400-e29b-41d4-a716-446655440000' AND status = 'succeeded' AND created_at <= '2026-09-03T12:30:00.000000123Z'::timestamptz AND retry = TRUE",
		rendered,
	)
}

func TestRender_DoesNotReplacePlaceholderLikeTextInsideSQLSyntax(t *testing.T) {
	t.Parallel()

	sql := "SELECT '$1', \"$2\", $$ $3 $$, $tag$ $4 $tag$, $1 -- $2\n/* $3 */"
	rendered, err := Render(sql, []any{"value"})
	require.NoError(t, err)
	require.Equal(
		t,
		"SELECT '$1', \"$2\", $$ $3 $$, $tag$ $4 $tag$, 'value' -- $2\n/* $3 */",
		rendered,
	)
}

func TestRender_HandlesMultiDigitPlaceholderWithoutPrefixCollision(t *testing.T) {
	t.Parallel()

	args := []any{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	rendered, err := Render("SELECT $10, $1", args)
	require.NoError(t, err)
	require.Equal(t, "SELECT 10, 1", rendered)
}

func TestRender_EscapesStringsAndRendersArrays(t *testing.T) {
	t.Parallel()

	rendered, err := Render(
		"SELECT * FROM contracts WHERE customer_name = $1 AND status = ANY($2)",
		[]any{"O'Reilly", []string{"active", "completed"}},
	)
	require.NoError(t, err)
	require.Equal(
		t,
		"SELECT * FROM contracts WHERE customer_name = 'O''Reilly' AND status = ANY(ARRAY['active', 'completed'])",
		rendered,
	)
}

func TestRender_RejectsMissingArgument(t *testing.T) {
	t.Parallel()

	_, err := Render("SELECT $2", []any{"only-one"})
	require.Error(t, err)
}

func TestRender_TypedNilUUIDPointerRendersAsNULL(t *testing.T) {
	t.Parallel()

	var actorID *uuid.UUID
	rendered, err := Render("INSERT INTO decisions (actor_id) VALUES ($1)", []any{actorID})
	require.NoError(t, err)
	require.Equal(t, "INSERT INTO decisions (actor_id) VALUES (NULL)", rendered)
}
