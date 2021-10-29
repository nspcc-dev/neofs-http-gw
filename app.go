package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math"
	"strconv"

	"github.com/fasthttp/router"
	"github.com/nspcc-dev/neo-go/cli/flags"
	"github.com/nspcc-dev/neo-go/cli/input"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
	"github.com/nspcc-dev/neofs-http-gw/downloader"
	"github.com/nspcc-dev/neofs-http-gw/uploader"
	"github.com/nspcc-dev/neofs-sdk-go/pkg/logger"
	"github.com/nspcc-dev/neofs-sdk-go/pkg/pool"
	"github.com/spf13/viper"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"google.golang.org/grpc/grpclog"
)

type (
	app struct {
		log          *zap.Logger
		pool         pool.Pool
		cfg          *viper.Viper
		auxiliaryLog logger.Logger
		webServer    *fasthttp.Server
		webDone      chan struct{}
	}

	// App is an interface for the main gateway function.
	App interface {
		Wait()
		Serve(context.Context)
	}

	// Option is an application option.
	Option func(a *app)
)

// WithLogger returns Option to set a specific logger.
func WithLogger(l *zap.Logger) Option {
	return func(a *app) {
		if l == nil {
			return
		}
		a.log = l
	}
}

// WithConfig returns Option to use specific Viper configuration.
func WithConfig(c *viper.Viper) Option {
	return func(a *app) {
		if c == nil {
			return
		}
		a.cfg = c
	}
}

func newApp(ctx context.Context, opt ...Option) App {
	var (
		key *ecdsa.PrivateKey
		err error
	)

	a := &app{
		log:       zap.L(),
		cfg:       viper.GetViper(),
		webServer: new(fasthttp.Server),
		webDone:   make(chan struct{}),
	}
	for i := range opt {
		opt[i](a)
	}
	a.auxiliaryLog = logger.GRPC(a.log)
	if a.cfg.GetBool(cmdVerbose) {
		grpclog.SetLoggerV2(a.auxiliaryLog)
	}
	// -- setup FastHTTP server --
	a.webServer.Name = "neofs-http-gw"
	a.webServer.ReadBufferSize = a.cfg.GetInt(cfgWebReadBufferSize)
	a.webServer.WriteBufferSize = a.cfg.GetInt(cfgWebWriteBufferSize)
	a.webServer.ReadTimeout = a.cfg.GetDuration(cfgWebReadTimeout)
	a.webServer.WriteTimeout = a.cfg.GetDuration(cfgWebWriteTimeout)
	a.webServer.DisableHeaderNamesNormalizing = true
	a.webServer.NoDefaultServerHeader = true
	a.webServer.NoDefaultContentType = true
	a.webServer.MaxRequestBodySize = a.cfg.GetInt(cfgWebMaxRequestBodySize)
	a.webServer.DisablePreParseMultipartForm = true
	a.webServer.StreamRequestBody = a.cfg.GetBool(cfgWebStreamRequestBody)
	// -- -- -- -- -- -- -- -- -- -- -- -- -- --
	key, err = getNeoFSKey(a)
	if err != nil {
		a.log.Fatal("failed to get neofs credentials", zap.Error(err))
	}
	pb := new(pool.Builder)
	for i := 0; ; i++ {
		address := a.cfg.GetString(cfgPeers + "." + strconv.Itoa(i) + ".address")
		weight := a.cfg.GetFloat64(cfgPeers + "." + strconv.Itoa(i) + ".weight")
		if address == "" {
			break
		}
		if weight <= 0 { // unspecified or wrong
			weight = 1
		}
		pb.AddNode(address, weight)
		a.log.Info("add connection", zap.String("address", address), zap.Float64("weight", weight))
	}
	opts := &pool.BuilderOptions{
		Key:                     key,
		NodeConnectionTimeout:   a.cfg.GetDuration(cfgConTimeout),
		NodeRequestTimeout:      a.cfg.GetDuration(cfgReqTimeout),
		ClientRebalanceInterval: a.cfg.GetDuration(cfgRebalance),
		SessionExpirationEpoch:  math.MaxUint64,
	}
	a.pool, err = pb.Build(ctx, opts)
	if err != nil {
		a.log.Fatal("failed to create connection pool", zap.Error(err))
	}
	return a
}

func getNeoFSKey(a *app) (*ecdsa.PrivateKey, error) {
	walletPath := a.cfg.GetString(cmdWallet)
	if len(walletPath) == 0 {
		a.log.Info("no wallet path specified, creating ephemeral key automatically for this run")
		return pool.NewEphemeralKey()
	}
	w, err := wallet.NewWalletFromFile(walletPath)
	if err != nil {
		return nil, err
	}

	var password *string
	if a.cfg.IsSet(cfgWalletPassphrase) {
		pwd := a.cfg.GetString(cfgWalletPassphrase)
		password = &pwd
	}
	return getKeyFromWallet(w, a.cfg.GetString(cmdAddress), password)
}

func getKeyFromWallet(w *wallet.Wallet, addrStr string, password *string) (*ecdsa.PrivateKey, error) {
	var addr util.Uint160
	var err error

	if addrStr == "" {
		addr = w.GetChangeAddress()
	} else {
		addr, err = flags.ParseAddress(addrStr)
		if err != nil {
			return nil, fmt.Errorf("invalid address")
		}
	}

	acc := w.GetAccount(addr)
	if acc == nil {
		return nil, fmt.Errorf("couldn't find wallet account for %s", addrStr)
	}

	if password == nil {
		pwd, err := input.ReadPassword("Enter password > ")
		if err != nil {
			return nil, fmt.Errorf("couldn't read password")
		}
		password = &pwd
	}

	if err := acc.Decrypt(*password, w.Scrypt); err != nil {
		return nil, fmt.Errorf("couldn't decrypt account: %w", err)
	}

	return &acc.PrivateKey().PrivateKey, nil
}

func (a *app) Wait() {
	a.log.Info("starting application")
	<-a.webDone // wait for web-server to be stopped
}

func (a *app) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		a.log.Info("shutting down web server", zap.Error(a.webServer.Shutdown()))
		close(a.webDone)
	}()
	edts := a.cfg.GetBool(cfgUploaderHeaderEnableDefaultTimestamp)
	uploader := uploader.New(a.log, a.pool, edts)
	downloader, err := downloader.New(a.log, downloader.Settings{ZipCompression: a.cfg.GetBool(cfgZipCompression)}, a.pool)
	if err != nil {
		a.log.Fatal("failed to create downloader", zap.Error(err))
	}
	// Configure router.
	r := router.New()
	r.RedirectTrailingSlash = true
	r.POST("/upload/{cid}", a.logger(uploader.Upload))
	a.log.Info("added path /upload/{cid}")
	r.GET("/get/{cid}/{oid}", a.logger(downloader.DownloadByAddress))
	r.HEAD("/get/{cid}/{oid}", a.logger(downloader.HeadByAddress))
	a.log.Info("added path /get/{cid}/{oid}")
	r.GET("/get_by_attribute/{cid}/{attr_key}/{attr_val:*}", a.logger(downloader.DownloadByAttribute))
	r.HEAD("/get_by_attribute/{cid}/{attr_key}/{attr_val:*}", a.logger(downloader.HeadByAttribute))
	a.log.Info("added path /get_by_attribute/{cid}/{attr_key}/{attr_val:*}")
	r.GET("/zip/{cid}/{prefix:*}", a.logger(downloader.DownloadZipped))
	a.log.Info("added path /zip/{cid}/{prefix}")
	// enable metrics
	if a.cfg.GetBool(cmdMetrics) {
		a.log.Info("added path /metrics/")
		attachMetrics(r, a.auxiliaryLog)
	}
	// enable pprof
	if a.cfg.GetBool(cmdPprof) {
		a.log.Info("added path /debug/pprof/")
		attachProfiler(r)
	}
	bind := a.cfg.GetString(cfgListenAddress)
	tlsCertPath := a.cfg.GetString(cfgTLSCertificate)
	tlsKeyPath := a.cfg.GetString(cfgTLSKey)

	a.webServer.Handler = r.Handler
	if tlsCertPath == "" && tlsKeyPath == "" {
		a.log.Info("running web server", zap.String("address", bind))
		err = a.webServer.ListenAndServe(bind)
	} else {
		a.log.Info("running web server (TLS-enabled)", zap.String("address", bind))
		err = a.webServer.ListenAndServeTLS(bind, tlsCertPath, tlsKeyPath)
	}
	if err != nil {
		a.log.Fatal("could not start server", zap.Error(err))
	}
}

func (a *app) logger(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return fasthttp.RequestHandler(func(ctx *fasthttp.RequestCtx) {
		a.log.Info("request", zap.String("remote", ctx.RemoteAddr().String()),
			zap.ByteString("method", ctx.Method()),
			zap.ByteString("path", ctx.Path()),
			zap.ByteString("query", ctx.QueryArgs().QueryString()),
			zap.Uint64("id", ctx.ID()))
		h(ctx)
	})
}
