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

func Context() context.Context {
	globalContextOnce.Do(func() {
		globalContext, _ = signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	})
	return globalContext
}
