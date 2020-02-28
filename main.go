package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"time"

	"github.com/fasthttp/router"
	"github.com/prometheus/client_golang/prometheus"
	http "github.com/prometheus/client_golang/prometheus/promhttp"
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

const (
	defaultHealthyMsg  = "NeoFS HTTP Gateway is "
	defaultContentType = "text/plain; charset=utf-8"
)

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

	r.GET("/-/ready", func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("NeoFS HTTP Gateway is ready")
	})

	r.GET("/-/healthy", func(c *fasthttp.RequestCtx) {
		code := fasthttp.StatusOK
		msg := "healthy"

		if err := a.pool.unhealthy.Load(); err != nil {
			msg = "unhealthy: " + err.Error()
			code = fasthttp.StatusBadRequest
		}

		c.Response.Reset()
		c.SetStatusCode(code)
		c.SetContentType(defaultContentType)
		c.SetBodyString(defaultHealthyMsg + msg)
	})

	// enable metrics
	if v.GetBool("metrics") {
		l.Info("enabled /metrics")
		r.GET("/metrics/", metricsHandler(prometheus.DefaultGatherer, http.HandlerOpts{
			ErrorLog: z.(http.Logger),
			//ErrorHandling:       0,
			//Registry:            nil,
			//DisableCompression:  false,
			//MaxRequestsInFlight: 0,
			//Timeout:             0,
			//EnableOpenMetrics:   false,
		}))
	}

	// enable pprof
	if v.GetBool("pprof") {
		l.Info("enabled /debug/pprof")
		r.GET("/debug/pprof/", pprofHandler())
		r.GET("/debug/pprof/:name", pprofHandler())
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
