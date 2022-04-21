package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"strconv"

	"github.com/fasthttp/router"
	"github.com/nspcc-dev/neo-go/cli/flags"
	"github.com/nspcc-dev/neo-go/cli/input"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
	"github.com/nspcc-dev/neofs-http-gw/downloader"
	"github.com/nspcc-dev/neofs-http-gw/response"
	"github.com/nspcc-dev/neofs-http-gw/uploader"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/spf13/viper"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type (
	app struct {
		log       *zap.Logger
		pool      *pool.Pool
		cfg       *viper.Viper
		webServer *fasthttp.Server
		webDone   chan struct{}
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

	var prm pool.InitParameters
	prm.SetKey(key)
	prm.SetNodeDialTimeout(a.cfg.GetDuration(cfgConTimeout))
	prm.SetHealthcheckTimeout(a.cfg.GetDuration(cfgReqTimeout))
	prm.SetClientRebalanceInterval(a.cfg.GetDuration(cfgRebalance))

	for i := 0; ; i++ {
		address := a.cfg.GetString(cfgPeers + "." + strconv.Itoa(i) + ".address")
		weight := a.cfg.GetFloat64(cfgPeers + "." + strconv.Itoa(i) + ".weight")
		priority := a.cfg.GetInt(cfgPeers + "." + strconv.Itoa(i) + ".priority")
		if address == "" {
			break
		}
		if weight <= 0 { // unspecified or wrong
			weight = 1
		}
		if priority <= 0 { // unspecified or wrong
			priority = 1
		}
		prm.AddNode(pool.NewNodeParam(priority, address, weight))
		a.log.Info("add connection", zap.String("address", address),
			zap.Float64("weight", weight), zap.Int("priority", priority))
	}

	a.pool, err = pool.NewPool(prm)
	if err != nil {
		a.log.Fatal("failed to create connection pool", zap.Error(err))
	}

	err = a.pool.Dial(ctx)
	if err != nil {
		a.log.Fatal("failed to dial pool", zap.Error(err))
	}
	return a
}

func getNeoFSKey(a *app) (*ecdsa.PrivateKey, error) {
	walletPath := a.cfg.GetString(cmdWallet)
	if len(walletPath) == 0 {
		walletPath = a.cfg.GetString(cfgWalletPath)
	}

	if len(walletPath) == 0 {
		a.log.Info("no wallet path specified, creating ephemeral key automatically for this run")
		key, err := keys.NewPrivateKey()
		if err != nil {
			return nil, err
		}
		return &key.PrivateKey, nil
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

	address := a.cfg.GetString(cmdAddress)
	if len(address) == 0 {
		address = a.cfg.GetString(cfgWalletAddress)
	}

	return getKeyFromWallet(w, address, password)
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
	a.log.Info("starting application", zap.String("app_name", "neofs-http-gw"), zap.String("version", Version))
	<-a.webDone // wait for web-server to be stopped
}

func (a *app) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		a.log.Info("shutting down web server", zap.Error(a.webServer.Shutdown()))
		close(a.webDone)
	}()
	edts := a.cfg.GetBool(cfgUploaderHeaderEnableDefaultTimestamp)
	uploader := uploader.New(ctx, a.log, a.pool, edts)
	downloader := downloader.New(ctx, a.log, downloader.Settings{ZipCompression: a.cfg.GetBool(cfgZipCompression)}, a.pool)
	// Configure router.
	r := router.New()
	r.RedirectTrailingSlash = true
	r.NotFound = func(r *fasthttp.RequestCtx) {
		response.Error(r, "Not found", fasthttp.StatusNotFound)
	}
	r.MethodNotAllowed = func(r *fasthttp.RequestCtx) {
		response.Error(r, "Method Not Allowed", fasthttp.StatusMethodNotAllowed)
	}
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
		attachMetrics(r, a.log)
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
	var err error
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
