package global

import (
	"context"
	"os/signal"
	"sync"
	"syscall"
)

var (
	globalContext     context.Context
	globalContextOnce sync.Once
)

// Context returns global context with initialized INT, TERM and HUP signal
// handlers set to notify this context.
func Context() context.Context {
	globalContextOnce.Do(func() {
		globalContext, _ = signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	})
	return globalContext
}
