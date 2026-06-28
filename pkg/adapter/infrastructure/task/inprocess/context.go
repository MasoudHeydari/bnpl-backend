package inprocess

import "context"

// mergeContexts returns a context canceled when either parent is canceled.
func mergeContexts(parents ...context.Context) (context.Context, context.CancelFunc) {
	merged, cancel := context.WithCancel(context.Background())

	for _, parent := range parents {
		if parent == nil {
			continue
		}

		if err := parent.Err(); err != nil {
			cancel()
			return merged, cancel
		}

		go func(parent context.Context) {
			select {
			case <-parent.Done():
				cancel()
			case <-merged.Done():
			}
		}(parent)
	}

	return merged, cancel
}
