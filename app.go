package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strconv"
	"time"

	"github.com/fasthttp/router"
	sdk "github.com/nspcc-dev/cdn-neofs-sdk"
	"github.com/nspcc-dev/cdn-neofs-sdk/creds/neofs"
	"github.com/nspcc-dev/cdn-neofs-sdk/logger"
	"github.com/nspcc-dev/cdn-neofs-sdk/pool"
	crypto "github.com/nspcc-dev/neofs-crypto"
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

	if a.cfg.GetBool("verbose") {
		grpclog.SetLoggerV2(a.wlog)
	}

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

	connections := make(map[string]float64)
	for i := 0; ; i++ {
		address := a.cfg.GetString("peers." + strconv.Itoa(i) + ".address")
		weight := a.cfg.GetFloat64("peers." + strconv.Itoa(i) + ".weight")
		if address == "" {
			break
		}

		connections[address] = weight
	}

	cred, err := prepareCredentials(a.cfg.GetString("key"), a.log)
	if err != nil {
		a.log.Fatal("could not prepare credentials", zap.Error(err))
	}

	a.pool, err = pool.New(ctx,
		pool.WithLogger(a.log),
		pool.WithCredentials(cred),
		pool.WithWeightPool(connections),
		pool.WithGRPCOptions(
			grpc.WithBlock(),
			grpc.WithInsecure(),
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:                a.cfg.GetDuration("keepalive.time"),
				Timeout:             a.cfg.GetDuration("keepalive.timeout"),
				PermitWithoutStream: a.cfg.GetBool("keepalive.permit_without_stream"),
			})))

	if err != nil {
		a.log.Fatal("could not prepare connection pool", zap.Error(err))
	}

	a.cli, err = sdk.New(ctx,
		sdk.WithLogger(a.log),
		sdk.WithCredentials(cred),
		sdk.WithConnectionPool(a.pool))
	if err != nil {
		a.log.Fatal("could not prepare sdk client", zap.Error(err))
	}

	return a
}

func prepareCredentials(key string, log *zap.Logger) (neofs.Credentials, error) {
	if key == generated {
		sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}

		key, err = crypto.WIFEncode(sk)
		if err != nil {
			return nil, err
		}

		log.Info("generate new key", zap.String("wif", key))
	}

	return neofs.New(key)
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
	r.GET("/get/{cid}/{oid}", a.receiveFile)

	// attaching /-/(ready,healthy)
	attachHealthy(r, a.pool.Status)

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
