package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"time"

	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"google.golang.org/grpc/grpclog"
)

type app struct {
	pool    *Pool
	log     *zap.Logger
	timeout time.Duration
	key     *ecdsa.PrivateKey
}

func main() {
	var (
		err error

		v = settings()
		l = newLogger(v)
		z = gRPCLogger(l)
		g = newGracefulContext(l)
		d = v.GetDuration("rebalance_timer")
	)

	if v.GetBool("verbose") {
		grpclog.SetLoggerV2(z)
	}

	a := &app{
		log:     l,
		pool:    newPool(l, v),
		key:     fetchKey(l, v),
		timeout: v.GetDuration("request_timeout"),
	}

	r := router.New()
	r.RedirectTrailingSlash = true
	r.GET("/get/:cid/:oid", a.receiveFile)

	// attaching /-/(ready,healthy)
	attachHealthy(r, a.pool.unhealthy)

	// enable metrics
	if v.GetBool("metrics") {
		l.Info("enabled /metrics")
		attachMetrics(r, z)
	}

	// enable pprof
	if v.GetBool("pprof") {
		l.Info("enabled /debug/pprof")
		attachProfiler(r)
	}

	en := &fasthttp.Server{
		Name:                          "neofs-http-gate",
		Handler:                       r.Handler,
		ReadBufferSize:                4096,
		ReadTimeout:                   time.Second * 15,
		GetOnly:                       true,
		DisableHeaderNamesNormalizing: true,
		NoDefaultServerHeader:         true,
		NoDefaultContentType:          true,
	}

	go func() {
		bind := v.GetString("listen_address")
		l.Info("run gateway server",
			zap.String("address", bind))

		if err := en.ListenAndServe(bind); err != nil {
			l.Panic("could not start server", zap.Error(err))
		}
	}()

	go checkConnection(g, d, a.pool)

	a.pool.reBalance(g)

	switch _, err = a.pool.getConnection(g); {
	case err == nil:
		// ignore
	case errors.Is(err, context.Canceled):
		// ignore
		// l.Info("context canceled")
	default:
		l.Error("could get connection", zap.Error(err))
		return
	}

	<-g.Done()

	l.Info("web server stopped", zap.Error(en.Shutdown()))
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

	p.close()
	tick.Stop()

	p.log.Info("connection worker stopped")
}
