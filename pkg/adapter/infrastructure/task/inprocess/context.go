package inprocess

import "context"

// mergeContexts returns a context canceled when either parent is canceled.
// context.AfterFunc registrations are stopped by the returned cancel function,
// so short-lived task executions do not leave watcher goroutines behind.
func mergeContexts(parents ...context.Context) (context.Context, context.CancelFunc) {
	merged, cancel := context.WithCancel(context.Background())
	stops := make([]func() bool, 0, len(parents))

	for _, parent := range parents {
		if parent == nil {
			continue
		}

		if parent.Err() != nil {
			cancel()
			break
		}

		stops = append(stops, context.AfterFunc(parent, cancel))
	}

	return merged, func() {
		for _, stop := range stops {
			stop()
		}

		cancel()
	}
}
