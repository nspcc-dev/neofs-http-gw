package main

import (
	"context"
	"crypto/ecdsa"
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
		v = settings()
		l = newLogger(v)
		g = newGracefulContext(l)
	)

	if v.GetBool("verbose") {
		grpclog.SetLoggerV2(gRPCLogger(l))
	}

	r := &router{
		log:     l,
		key:     fetchKey(l, v),
		pool:    newPool(g, l, v),
		timeout: v.GetDuration("request_timeout"),
	}

	go checkConnection(g, r.pool)

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

		if err := e.Start(v.GetString("listen_address")); err != nil {
			l.Panic("could not start server", zap.Error(err))
		}
	}()

	<-g.Done()

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*30)
	defer cancel()

	l.Info("stopping server", zap.Error(e.Shutdown(ctx)))
}

func checkConnection(ctx context.Context, p *Pool) {
	tick := time.NewTicker(time.Second * 15)

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-tick.C:
			p.reBalance(ctx)
		}
	}

	tick.Stop()

	p.log.Info("stop connection worker")
}
