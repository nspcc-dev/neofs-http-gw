package global

import (
	"context"
	"os/signal"
	"sync"
	"syscall"
)

var (
	globalContext    context.Context
	globalContexOnce sync.Once
)

func Context() context.Context {
	globalContexOnce.Do(func() {
		globalContext, _ = signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	})
	return globalContext
}
