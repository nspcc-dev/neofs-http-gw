package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/nspcc-dev/neofs-sdk-go/pkg/logger"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func main() {
	var (
		v = settings()
		l = newLogger(v)
	)
	globalContext, _ := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	app := newApp(globalContext, WithLogger(l), WithConfig(v))
	go app.Serve(globalContext)
	app.Wait()
}

func newLogger(v *viper.Viper) *zap.Logger {
	options := []logger.Option{
		logger.WithLevel(v.GetString(cfgLoggerLevel)),
		logger.WithTraceLevel(v.GetString(cfgLoggerTraceLevel)),
		logger.WithFormat(v.GetString(cfgLoggerFormat)),
		logger.WithSamplingInitial(v.GetInt(cfgLoggerSamplingInitial)),
		logger.WithSamplingThereafter(v.GetInt(cfgLoggerSamplingThereafter)),
		logger.WithAppName(v.GetString(cfgApplicationName)),
		logger.WithAppVersion(v.GetString(cfgApplicationVersion)),
	}
	if v.GetBool(cfgLoggerNoCaller) {
		options = append(options, logger.WithoutCaller())
	}
	if v.GetBool(cfgLoggerNoDisclaimer) {
		options = append(options, logger.WithoutDisclaimer())
	}
	l, err := logger.New(options...)
	if err != nil {
		panic(err)
	}
	return l
}
