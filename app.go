package main

import (
	"context"
	"sort"
	"strconv"

	"github.com/fasthttp/router"
	"github.com/nspcc-dev/neofs-api-go/pkg/client"
	"github.com/nspcc-dev/neofs-api-go/pkg/token"
	"github.com/nspcc-dev/neofs-http-gate/logger"
	"github.com/nspcc-dev/neofs-http-gate/neofs"
	"github.com/nspcc-dev/neofs-http-gate/uploader"
	"github.com/spf13/viper"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"google.golang.org/grpc/grpclog"
)

type (
	app struct {
		plant         neofs.ClientPlant
		getOperations struct {
			client       client.Client
			sessionToken *token.SessionToken
		}
		log                    *zap.Logger
		cfg                    *viper.Viper
		wlog                   logger.Logger
		web                    *fasthttp.Server
		jobDone                chan struct{}
		webDone                chan struct{}
		enableDefaultTimestamp bool
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

	a.enableDefaultTimestamp = a.cfg.GetBool(cfgUploaderHeaderEnableDefaultTimestamp)

	a.wlog = logger.GRPC(a.log)

	if a.cfg.GetBool(cmdVerbose) {
		grpclog.SetLoggerV2(a.wlog)
	}

	// conTimeout := a.cfg.GetDuration(cfgConTimeout)
	// reqTimeout := a.cfg.GetDuration(cfgReqTimeout)
	// tckTimeout := a.cfg.GetDuration(cfgRebalance)

	// -- setup FastHTTP server --
	a.web.Name = "neofs-http-gate"
	a.web.ReadBufferSize = a.cfg.GetInt(cfgWebReadBufferSize)
	a.web.WriteBufferSize = a.cfg.GetInt(cfgWebWriteBufferSize)
	a.web.ReadTimeout = a.cfg.GetDuration(cfgWebReadTimeout)
	a.web.WriteTimeout = a.cfg.GetDuration(cfgWebWriteTimeout)
	a.web.DisableHeaderNamesNormalizing = true
	a.web.NoDefaultServerHeader = true
	a.web.NoDefaultContentType = true
	a.web.MaxRequestBodySize = a.cfg.GetInt(cfgWebMaxRequestBodySize)

	// -- -- -- -- -- -- FIXME -- -- -- -- -- --
	// Does not work with StreamRequestBody,
	// some bugs with readMultipartForm
	// https://github.com/valyala/fasthttp/issues/968
	a.web.DisablePreParseMultipartForm = true
	a.web.StreamRequestBody = a.cfg.GetBool(cfgWebStreamRequestBody)
	// -- -- -- -- -- -- -- -- -- -- -- -- -- --

	var cl connectionList
	for i := 0; ; i++ {
		address := a.cfg.GetString(cfgPeers + "." + strconv.Itoa(i) + ".address")
		weight := a.cfg.GetFloat64(cfgPeers + "." + strconv.Itoa(i) + ".weight")
		if address == "" {
			break
		}
		cl = append(cl, connection{address: address, weight: weight})
		a.log.Info("add connection peer", zap.String("address", address), zap.Float64("weight", weight))
	}
	sort.Sort(sort.Reverse(cl))
	cred, err := neofs.NewCredentials(a.cfg.GetString(cmdNeoFSKey))
	if err != nil {
		a.log.Fatal("could not get credentials", zap.Error(err))
	}
	a.plant, err = neofs.NewClientPlant(ctx, cl[0].address, cred)
	if err != nil {
		a.log.Fatal("failed to create neofs client")
	}
	a.getOperations.client, a.getOperations.sessionToken, err = a.plant.GetReusableArtifacts(ctx)
	if err != nil {
		a.log.Fatal("failed to get neofs client's reusable artifacts")
	}
	return a
}

func (a *app) Wait() {
	a.log.Info("starting application")

	select {
	case <-a.jobDone: // wait for job is stopped
		<-a.webDone
	case <-a.webDone: // wait for web-server is stopped
		<-a.jobDone
	}
}

func (a *app) Worker(ctx context.Context) {
	close(a.jobDone)
}

func (a *app) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		a.log.Info("shutting down web server", zap.Error(a.web.Shutdown()))
		close(a.webDone)
	}()
	uploader := uploader.New(a.log, a.plant, a.enableDefaultTimestamp)
	// Configure router.
	r := router.New()
	r.RedirectTrailingSlash = true
	r.POST("/upload/{cid}", uploader.Upload)
	a.log.Info("added path /upload/{cid}")
	r.GET("/get/{cid}/{oid}", a.byAddress)
	a.log.Info("added path /get/{cid}/{oid}")
	r.GET("/get_by_attribute/{cid}/{attr_key}/{attr_val:*}", a.byAttribute)
	a.log.Info("added path /get_by_attribute/{cid}/{attr_key}/{attr_val:*}")
	// attaching /-/(ready,healthy)
	// attachHealthy(r, a.pool.Status)
	// enable metrics
	if a.cfg.GetBool(cmdMetrics) {
		a.log.Info("added path /metrics/")
		attachMetrics(r, a.wlog)
	}
	// enable pprof
	if a.cfg.GetBool(cmdPprof) {
		a.log.Info("added path /debug/pprof/")
		attachProfiler(r)
	}
	bind := a.cfg.GetString(cfgListenAddress)
	a.log.Info("running web server", zap.String("address", bind))
	a.web.Handler = r.Handler
	if err := a.web.ListenAndServe(bind); err != nil {
		a.log.Fatal("could not start server", zap.Error(err))
	}
}

type connection struct {
	address string
	weight  float64
}

type connectionList []connection

func (p connectionList) Len() int           { return len(p) }
func (p connectionList) Less(i, j int) bool { return p[i].weight < p[j].weight }
func (p connectionList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
