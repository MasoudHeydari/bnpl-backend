package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

// Route sends messages whose destination starts with Prefix to Publisher.
type Route struct {
	// Prefix is the destination scheme prefix, for example "webhook://".
	Prefix string

	// Publisher receives matching messages.
	Publisher messaging.Publisher
}

// Router is a Publisher that selects an underlying Publisher by destination
// prefix and falls back to a default publisher (for example Kafka) otherwise.
//
// The outbox publisher keeps a single Publisher dependency, so adding a new
// transport only means registering another route.
type Router struct {
	routes   []Route
	fallback messaging.Publisher
}

var _ messaging.Publisher = (*Router)(nil)

// New constructs a destination-prefix router. The fallback and every route
// publisher are required, and prefixes must be non-empty.
func New(fallback messaging.Publisher, routes ...Route) (*Router, error) {
	const op = "messagingrouter.New"

	if fallback == nil {
		return nil, fmt.Errorf("%s: fallback publisher is required", op)
	}

	for i, route := range routes {
		if strings.TrimSpace(route.Prefix) == "" {
			return nil, fmt.Errorf("%s: route %d prefix is required", op, i)
		}

		if route.Publisher == nil {
			return nil, fmt.Errorf("%s: route %d publisher is required", op, i)
		}
	}

	return &Router{routes: append([]Route(nil), routes...), fallback: fallback}, nil
}

// Publish forwards msg to the first route whose prefix matches topic, otherwise
// to the fallback publisher.
func (r *Router) Publish(ctx context.Context, topic string, msg messaging.Message) error {
	for _, route := range r.routes {
		if strings.HasPrefix(topic, route.Prefix) {
			return route.Publisher.Publish(ctx, topic, msg)
		}
	}

	return r.fallback.Publish(ctx, topic, msg)
}

// Close is a no-op: the router does not own the publishers it dispatches to.
// Infrastructure owns the messaging fallback; External owns webhook HTTP
// deliverer resources when that route is configured.
func (r *Router) Close() error { return nil }
