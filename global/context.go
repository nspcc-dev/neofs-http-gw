package global

import (
	"context"
	"os/signal"
	"sync"
	"syscall"
)

var (
	globalContext        context.Context
	globalContextOnce    sync.Once
	globalContextBarrier = make(chan struct{})
)

func Context() context.Context {
	globalContextOnce.Do(func() {
		globalContext, _ = signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		close(globalContextBarrier)
	})
	<-globalContextBarrier
	return globalContext
}
