//go:build !linux

package catalog

// runLowPriority runs fn (priorities are only lowered on Linux).
func runLowPriority(fn func() error) error { return fn() }
