package model

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidStatusTransition is returned when a status transition is not allowed.
	ErrInvalidStatusTransition = errors.New("invalid status transition")
)

type statusTransitionTable[S ~string] struct {
	valid      map[S]struct{}
	allowed    map[S]map[S]struct{}
	terminal   map[S]struct{}
	statusKind string
}

func newStatusTransitionTable[S ~string](
	statusKind string,
	transitions map[S][]S,
	terminal []S,
) statusTransitionTable[S] {
	valid := make(map[S]struct{})
	allowed := make(map[S]map[S]struct{})
	term := make(map[S]struct{})

	for from, targets := range transitions {
		valid[from] = struct{}{}
		if allowed[from] == nil {
			allowed[from] = make(map[S]struct{})
		}

		for _, to := range targets {
			valid[to] = struct{}{}
			allowed[from][to] = struct{}{}
		}
	}

	for _, status := range terminal {
		valid[status] = struct{}{}
		term[status] = struct{}{}
	}

	return statusTransitionTable[S]{
		valid:      valid,
		allowed:    allowed,
		terminal:   term,
		statusKind: statusKind,
	}
}

func (t statusTransitionTable[S]) isValid(status S) bool {
	_, ok := t.valid[status]
	return ok
}

func (t statusTransitionTable[S]) canTransitionTo(current, next S) bool {
	if current == next {
		return t.isValid(current)
	}

	if !t.isValid(current) || !t.isValid(next) {
		return false
	}

	targets, ok := t.allowed[current]
	if !ok {
		return false
	}

	_, ok = targets[next]
	return ok
}

func (t statusTransitionTable[S]) validateTransitionTo(current, next S) error {
	const op = "model.statusTransitionTable.validateTransitionTo"

	if current == next {
		if !t.isValid(current) {
			return fmt.Errorf("%s: invalid %s: %q", op, t.statusKind, current)
		}

		return nil
	}

	if !t.isValid(current) {
		return fmt.Errorf("%s: invalid %s: %q", op, t.statusKind, current)
	}

	if !t.isValid(next) {
		return fmt.Errorf("%s: invalid %s: %q", op, t.statusKind, next)
	}

	if !t.canTransitionTo(current, next) {
		return fmt.Errorf(
			"%s: %w: cannot transition %s from %q to %q", op, ErrInvalidStatusTransition,
			t.statusKind,
			current,
			next,
		)
	}

	return nil
}

func (t statusTransitionTable[S]) isTerminal(status S) bool {
	_, ok := t.terminal[status]
	return ok
}
