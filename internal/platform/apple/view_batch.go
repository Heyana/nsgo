//go:build darwin

package apple

// StartImages starts the current views queued by a mount batch and clears
// the retained queue before reusing its storage for the next batch.
func StartImages[V any](pending []V, scale float64, current func(V) bool, start func(V, float64)) []V {
	for _, v := range pending {
		if current(v) {
			start(v, scale)
		}
	}
	clear(pending)
	return pending[:0]
}
