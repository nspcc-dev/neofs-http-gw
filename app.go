package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"time"

	"github.com/fasthttp/router"

	"google.golang.org/grpc/grpclog"

	"github.com/nspcc-dev/neofs-api-go/service"
	"github.com/nspcc-dev/neofs-api-go/state"
	"github.com/spf13/viper"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type (
	app struct {
		pool *Pool
		log  *zap.Logger
		cfg  *viper.Viper
		key  *ecdsa.PrivateKey

		wlog logger
		web  *fasthttp.Server

		jobDone chan struct{}
		webDone chan struct{}

		rebalanceTimer time.Duration

		nodes []string

		reqHealth *state.HealthRequest
		reqNetmap *state.NetmapRequest

		conTimeout time.Duration
		reqTimeout time.Duration
	}

	App interface {
		Wait()
		Worker(context.Context)
		Serve(context.Context)
	}

	Option func(a *app)
)

func WithLogger(l *zap.Logger) Option {
	return func(a *app) {
		if l == nil {
			return
		}
		a.log = l
	}
}

func WithConfig(c *viper.Viper) Option {
	return func(a *app) {
		if c == nil {
			return
		}
		a.cfg = c
	}
}

func newApp(opt ...Option) App {
	a := &app{
		log: zap.L(),
		cfg: viper.GetViper(),
		web: new(fasthttp.Server),

		jobDone: make(chan struct{}),
		webDone: make(chan struct{}),
	}

	for i := range opt {
		opt[i](a)
	}

	a.wlog = gRPCLogger(a.log)

	if a.cfg.GetBool("verbose") {
		grpclog.SetLoggerV2(a.wlog)
	}

	a.key = fetchKey(a.log, a.cfg)
	a.rebalanceTimer = a.cfg.GetDuration("rebalance_timer")
	a.conTimeout = a.cfg.GetDuration("connect_timeout")
	a.reqTimeout = a.cfg.GetDuration("request_timeout")

	// -- setup FastHTTP server: --
	a.web.Name = "neofs-http-gate"
	a.web.ReadBufferSize = a.cfg.GetInt("web.read_buffer_size")
	a.web.WriteBufferSize = a.cfg.GetInt("web.write_buffer_size")
	a.web.ReadTimeout = a.cfg.GetDuration("web.read_timeout")
	a.web.WriteTimeout = a.cfg.GetDuration("web.write_timeout")
	a.web.GetOnly = true
	a.web.DisableHeaderNamesNormalizing = true
	a.web.NoDefaultServerHeader = true
	a.web.NoDefaultContentType = true
	// -- -- -- -- -- -- -- -- -- --

	a.reqHealth = new(state.HealthRequest)
	a.reqHealth.SetTTL(service.NonForwardingTTL)

	if err := service.SignRequestHeader(a.key, a.reqHealth); err != nil {
		a.log.Fatal("could not sign `HealthRequest`", zap.Error(err))
	}

	a.reqNetmap = new(state.NetmapRequest)
	a.reqNetmap.SetTTL(service.SingleForwardingTTL)

	if err := service.SignRequestHeader(a.key, a.reqNetmap); err != nil {
		a.log.Fatal("could not sign `NetmapRequest`", zap.Error(err))
	}

	a.pool = newPool(a.log, a.cfg)

	return a
}

func (a *app) Wait() {
	a.log.Info("application started")

	select {
	case <-a.jobDone: // wait for job is stopped
		<-a.webDone
	case <-a.webDone: // wait for web-server is stopped
		<-a.jobDone
	}
}

func (a *app) Worker(ctx context.Context) {
	dur := a.rebalanceTimer
	tick := time.NewTimer(dur)

	a.pool.reBalance(ctx)

	switch _, err := a.pool.getConnection(ctx); {
	case err == nil:
		// ignore
	case errors.Is(err, context.Canceled):
		// ignore
		// l.Info("context canceled")
	default:
		a.log.Fatal("could get connection", zap.Error(err))
	}

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-tick.C:
			a.pool.reBalance(ctx)
			tick.Reset(dur)
		}
	}

	a.pool.close()
	tick.Stop()

	a.log.Info("connection worker stopped")

	close(a.jobDone)
}

func (a *app) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		a.log.Info("stop web-server", zap.Error(a.web.Shutdown()))
		close(a.webDone)
	}()

	r := router.New()
	r.RedirectTrailingSlash = true

	a.log.Info("enabled /get/{cid}/{oid}")
	r.GET("/get/{cid}/{oid}", a.receiveFile)

	// attaching /-/(ready,healthy)
	attachHealthy(r, a.pool.unhealthy)

	// enable metrics
	if a.cfg.GetBool("metrics") {
		a.log.Info("enabled /metrics/")
		attachMetrics(r, a.wlog)
	}

	// enable pprof
	if a.cfg.GetBool("pprof") {
		a.log.Info("enabled /debug/pprof/")
		attachProfiler(r)
	}

	bind := a.cfg.GetString("listen_address")
	a.log.Info("run gateway server",
		zap.String("address", bind))

	a.web.Handler = r.Handler
	if err := a.web.ListenAndServe(bind); err != nil {
		a.log.Fatal("could not start server", zap.Error(err))
	}
}
