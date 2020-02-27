package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"google.golang.org/grpc/grpclog"
)

type router struct {
	pool    *Pool
	log     *zap.Logger
	timeout time.Duration
	key     *ecdsa.PrivateKey
}

func main() {
	var (
		err  error
		pool *Pool

		v = settings()
		l = newLogger(v)
		g = newGracefulContext(l)
		d = v.GetDuration("rebalance_timer")
	)

	if v.GetBool("verbose") {
		grpclog.SetLoggerV2(gRPCLogger(l))
	}

	switch pool, err = newPool(g, l, v); {
	case err == nil:
		// ignore
	case errors.Is(err, context.Canceled):
		l.Info("close application")
		return
	default:
		l.Error("could get connection", zap.Error(err))
		return
	}

	r := &router{
		log:     l,
		pool:    pool,
		key:     fetchKey(l, v),
		timeout: v.GetDuration("request_timeout"),
	}

	go checkConnection(g, d, r.pool)

	e := echo.New()
	e.Debug = false
	e.HidePort = true
	e.HideBanner = true

	e.GET("/:cid/:oid", r.receiveFile)

	// enable metrics
	if v.GetBool("metrics") {
		l.Info("enabled /metrics")
		e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
	}

	// enable pprof
	if v.GetBool("pprof") {
		l.Info("enabled /debug/pprof")
		e.Any("/debug/pprof*", echo.WrapHandler(http.DefaultServeMux))
	}

	go func() {
		l.Info("run gateway server",
			zap.String("address", v.GetString("listen_address")))

		if err := e.Start(v.GetString("listen_address")); err != nil && !errors.Is(err, http.ErrServerClosed) {
			l.Panic("could not start server", zap.Error(err))
		}
	}()

	<-g.Done()

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*30)
	defer cancel()

	l.Info("stopping server", zap.Error(e.Shutdown(ctx)))
}

func checkConnection(ctx context.Context, dur time.Duration, p *Pool) {
	tick := time.NewTimer(dur)

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-tick.C:
			p.reBalance(ctx)
			tick.Reset(dur)
		}
	}

	tick.Stop()

	p.log.Info("stop connection worker")
}
