package main

import (
	"context"
	"crypto/ecdsa"
	"strconv"

	"github.com/fasthttp/router"
	sdk "github.com/nspcc-dev/cdn-sdk"
	"github.com/nspcc-dev/cdn-sdk/creds/neofs"
	"github.com/nspcc-dev/cdn-sdk/logger"
	"github.com/nspcc-dev/cdn-sdk/pool"
	"github.com/spf13/viper"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/keepalive"
)

type (
	app struct {
		cli  sdk.Client
		pool pool.Client
		log  *zap.Logger
		cfg  *viper.Viper
		key  *ecdsa.PrivateKey

		wlog logger.Logger
		web  *fasthttp.Server

		jobDone chan struct{}
		webDone chan struct{}
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

func newApp(ctx context.Context, opt ...Option) App {
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

	a.wlog = logger.GRPC(a.log)

	if a.cfg.GetBool(cmdVerbose) {
		grpclog.SetLoggerV2(a.wlog)
	}

	conTimeout := a.cfg.GetDuration(cfgConTimeout)
	reqTimeout := a.cfg.GetDuration(cfgReqTimeout)
	tckTimeout := a.cfg.GetDuration(cfgRebalance)

	// -- setup FastHTTP server: --
	a.web.Name = "neofs-http-gate"
	a.web.ReadBufferSize = a.cfg.GetInt(cfgWebReadBufferSize)
	a.web.WriteBufferSize = a.cfg.GetInt(cfgWebWriteBufferSize)
	a.web.ReadTimeout = a.cfg.GetDuration(cfgWebReadTimeout)
	a.web.WriteTimeout = a.cfg.GetDuration(cfgWebWriteTimeout)
	a.web.GetOnly = true
	a.web.DisableHeaderNamesNormalizing = true
	a.web.NoDefaultServerHeader = true
	a.web.NoDefaultContentType = true
	// -- -- -- -- -- -- -- -- -- --

	connections := make(map[string]float64)
	for i := 0; ; i++ {
		address := a.cfg.GetString(cfgPeers + "." + strconv.Itoa(i) + ".address")
		weight := a.cfg.GetFloat64(cfgPeers + "." + strconv.Itoa(i) + ".weight")
		if address == "" {
			break
		}

		connections[address] = weight
		a.log.Info("add connection peer",
			zap.String("address", address),
			zap.Float64("weight", weight))
	}

	cred, err := neofs.New(a.cfg.GetString(cmdNeoFSKey))
	if err != nil {
		a.log.Fatal("could not prepare credentials", zap.Error(err))
	}

	a.pool, err = pool.New(ctx,
		pool.WithLogger(a.log),
		pool.WithCredentials(cred),
		pool.WithWeightPool(connections),
		pool.WithTickerTimeout(tckTimeout),
		pool.WithConnectTimeout(conTimeout),
		pool.WithRequestTimeout(reqTimeout),
		pool.WithAPIPreparer(sdk.APIPreparer),
		pool.WithGRPCOptions(
			grpc.WithBlock(),
			grpc.WithInsecure(),
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:                a.cfg.GetDuration(cfgKeepaliveTime),
				Timeout:             a.cfg.GetDuration(cfgKeepaliveTimeout),
				PermitWithoutStream: a.cfg.GetBool(cfgKeepalivePermitWithoutStream),
			})))

	if err != nil {
		a.log.Fatal("could not prepare connection pool", zap.Error(err))
	}

	a.cli, err = sdk.New(ctx,
		sdk.WithLogger(a.log),
		sdk.WithCredentials(cred),
		sdk.WithConnectionPool(a.pool),
		sdk.WithAPIPreparer(sdk.APIPreparer))
	if err != nil {
		a.log.Fatal("could not prepare sdk client", zap.Error(err))
	}

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
	a.pool.Worker(ctx)
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
	r.GET("/get/{cid}/{oid}", a.byAddress)
	a.log.Info("enabled /get_by_attribute/{cid}/{attr_key}/{attr_val}")
	r.GET("/get_by_attribute/{cid}/{attr_key}/{attr_val}", a.byAttribute)

	// attaching /-/(ready,healthy)
	attachHealthy(r, a.pool.Status)

	// enable metrics
	if a.cfg.GetBool(cmdMetrics) {
		a.log.Info("enabled /metrics/")
		attachMetrics(r, a.wlog)
	}

	// enable pprof
	if a.cfg.GetBool(cmdPprof) {
		a.log.Info("enabled /debug/pprof/")
		attachProfiler(r)
	}

	bind := a.cfg.GetString(cfgListenAddress)
	a.log.Info("run gateway server",
		zap.String("address", bind))

	a.web.Handler = r.Handler
	if err := a.web.ListenAndServe(bind); err != nil {
		a.log.Fatal("could not start server", zap.Error(err))
	}
}
