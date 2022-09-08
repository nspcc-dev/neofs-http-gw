package main

import (
	"context"
	"os/signal"
	"syscall"
)

func main() {
	globalContext, _ := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	v := settings()
	logger, atomicLevel := newLogger(v)

	application := newApp(globalContext, WithLogger(logger, atomicLevel), WithConfig(v))
	go application.Serve(globalContext)
	application.Wait()
}
