package main

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nspcc-dev/neofs-http-gw/resolver"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/valyala/fasthttp"
)

const (
	defaultRebalanceTimer = 60 * time.Second
	defaultRequestTimeout = 15 * time.Second
	defaultConnectTimeout = 10 * time.Second

	defaultShutdownTimeout = 15 * time.Second

	defaultPoolErrorThreshold uint32 = 100

	cfgListenAddress  = "listen_address"
	cfgTLSCertificate = "tls_certificate"
	cfgTLSKey         = "tls_key"

	// Web.
	cfgWebReadBufferSize     = "web.read_buffer_size"
	cfgWebWriteBufferSize    = "web.write_buffer_size"
	cfgWebReadTimeout        = "web.read_timeout"
	cfgWebWriteTimeout       = "web.write_timeout"
	cfgWebStreamRequestBody  = "web.stream_request_body"
	cfgWebMaxRequestBodySize = "web.max_request_body_size"

	// Metrics / Profiler.
	cfgPrometheusEnabled = "prometheus.enabled"
	cfgPrometheusAddress = "prometheus.address"
	cfgPprofEnabled      = "pprof.enabled"
	cfgPprofAddress      = "pprof.address"

	// Pool config.
	cfgConTimeout         = "connect_timeout"
	cfgReqTimeout         = "request_timeout"
	cfgRebalance          = "rebalance_timer"
	cfgPoolErrorThreshold = "pool_error_threshold"

	// Logger.
	cfgLoggerLevel = "logger.level"

	// Wallet.
	cfgWalletPassphrase = "wallet.passphrase"
	cfgWalletPath       = "wallet.path"
	cfgWalletAddress    = "wallet.address"

	// Uploader Header.
	cfgUploaderHeaderEnableDefaultTimestamp = "upload_header.use_default_timestamp"

	// Peers.
	cfgPeers = "peers"

	// NeoGo.
	cfgRPCEndpoint = "rpc_endpoint"

	// Resolving.
	cfgResolveOrder = "resolve_order"

	// Zip compression.
	cfgZipCompression = "zip.compression"

	// Command line args.
	cmdHelp    = "help"
	cmdVersion = "version"
	cmdPprof   = "pprof"
	cmdMetrics = "metrics"
	cmdWallet  = "wallet"
	cmdAddress = "address"
	cmdConfig  = "config"
)

var ignore = map[string]struct{}{
	cfgPeers:   {},
	cmdHelp:    {},
	cmdVersion: {},
}

func settings() *viper.Viper {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvPrefix(Prefix)
	v.AllowEmptyEnv(true)
	v.SetConfigType("yaml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// flags setup:
	flags := pflag.NewFlagSet("commandline", pflag.ExitOnError)
	flags.SetOutput(os.Stdout)
	flags.SortFlags = false

	flags.Bool(cmdPprof, false, "enable pprof")
	flags.Bool(cmdMetrics, false, "enable prometheus")

	help := flags.BoolP(cmdHelp, "h", false, "show help")
	version := flags.BoolP(cmdVersion, "v", false, "show version")

	flags.StringP(cmdWallet, "w", "", `path to the wallet`)
	flags.String(cmdAddress, "", `address of wallet account`)
	config := flags.String(cmdConfig, "", "config path")
	flags.Duration(cfgConTimeout, defaultConnectTimeout, "gRPC connect timeout")
	flags.Duration(cfgReqTimeout, defaultRequestTimeout, "gRPC request timeout")
	flags.Duration(cfgRebalance, defaultRebalanceTimer, "gRPC connection rebalance timer")

	flags.String(cfgListenAddress, "0.0.0.0:8082", "address to listen")
	flags.String(cfgTLSCertificate, "", "TLS certificate path")
	flags.String(cfgTLSKey, "", "TLS key path")
	peers := flags.StringArrayP(cfgPeers, "p", nil, "NeoFS nodes")

	resolveMethods := flags.StringSlice(cfgResolveOrder, []string{resolver.NNSResolver, resolver.DNSResolver}, "set container name resolve order")

	// set defaults:

	// logger:
	v.SetDefault(cfgLoggerLevel, "debug")

	// pool:
	v.SetDefault(cfgPoolErrorThreshold, defaultPoolErrorThreshold)

	// web-server:
	v.SetDefault(cfgWebReadBufferSize, 4096)
	v.SetDefault(cfgWebWriteBufferSize, 4096)
	v.SetDefault(cfgWebReadTimeout, time.Minute*10)
	v.SetDefault(cfgWebWriteTimeout, time.Minute*5)
	v.SetDefault(cfgWebStreamRequestBody, true)
	v.SetDefault(cfgWebMaxRequestBodySize, fasthttp.DefaultMaxRequestBodySize)

	// upload header
	v.SetDefault(cfgUploaderHeaderEnableDefaultTimestamp, false)

	// zip:
	v.SetDefault(cfgZipCompression, false)

	// metrics
	v.SetDefault(cfgPprofAddress, "localhost:8083")
	v.SetDefault(cfgPrometheusAddress, "localhost:8084")

	// Binding flags
	if err := v.BindPFlag(cfgPprofEnabled, flags.Lookup(cmdPprof)); err != nil {
		panic(err)
	}
	if err := v.BindPFlag(cfgPrometheusEnabled, flags.Lookup(cmdMetrics)); err != nil {
		panic(err)
	}

	if err := v.BindPFlags(flags); err != nil {
		panic(err)
	}

	if err := flags.Parse(os.Args); err != nil {
		panic(err)
	}

	if resolveMethods != nil {
		v.SetDefault(cfgResolveOrder, *resolveMethods)
	}

	switch {
	case help != nil && *help:
		fmt.Printf("NeoFS HTTP Gateway %s\n", Version)
		flags.PrintDefaults()

		fmt.Println()
		fmt.Println("Default environments:")
		fmt.Println()
		keys := v.AllKeys()
		sort.Strings(keys)

		for i := range keys {
			if _, ok := ignore[keys[i]]; ok {
				continue
			}

			k := strings.Replace(keys[i], ".", "_", -1)
			fmt.Printf("%s_%s = %v\n", Prefix, strings.ToUpper(k), v.Get(keys[i]))
		}

		fmt.Println()
		fmt.Println("Peers preset:")
		fmt.Println()

		fmt.Printf("%s_%s_[N]_ADDRESS = string\n", Prefix, strings.ToUpper(cfgPeers))
		fmt.Printf("%s_%s_[N]_WEIGHT = float\n", Prefix, strings.ToUpper(cfgPeers))

		os.Exit(0)
	case version != nil && *version:
		fmt.Printf("NeoFS HTTP Gateway\nVersion: %s\nGoVersion: %s\n", Version, runtime.Version())
		os.Exit(0)
	}

	if v.IsSet(cmdConfig) {
		if cfgFile, err := os.Open(*config); err != nil {
			panic(err)
		} else if err := v.ReadConfig(cfgFile); err != nil {
			panic(err)
		}
	}

	if peers != nil && len(*peers) > 0 {
		for i := range *peers {
			v.SetDefault(cfgPeers+"."+strconv.Itoa(i)+".address", (*peers)[i])
			v.SetDefault(cfgPeers+"."+strconv.Itoa(i)+".weight", 1)
			v.SetDefault(cfgPeers+"."+strconv.Itoa(i)+".priority", 1)
		}
	}

	return v
}
