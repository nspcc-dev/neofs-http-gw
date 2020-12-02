package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type empty int

const (
	devNull   = empty(0)
	generated = "generated"

	minimumTTLInMinutes = 5

	defaultTTL = minimumTTLInMinutes * time.Minute

	defaultRebalanceTimer = 15 * time.Second
	defaultRequestTimeout = 15 * time.Second
	defaultConnectTimeout = 30 * time.Second

	defaultKeepaliveTime    = 10 * time.Second
	defaultKeepaliveTimeout = 10 * time.Second

	// Logger:
	cfgLoggerLevel              = "logger.level"
	cfgLoggerFormat             = "logger.format"
	cfgLoggerTraceLevel         = "logger.trace_level"
	cfgLoggerNoCaller           = "logger.no_caller"
	cfgLoggerNoDisclaimer       = "logger.no_disclaimer"
	cfgLoggerSamplingInitial    = "logger.sampling.initial"
	cfgLoggerSamplingThereafter = "logger.sampling.thereafter"

	// Peers
	cfgPeers = "peers"

	// Application
	cfgApplicationName      = "app.name"
	cfgApplicationVersion   = "app.version"
	cfgApplicationBuildTime = "app.build_time"

	// command line args
	cmdHelp    = "help"
	cmdVersion = "version"
)

var ignore = map[string]struct{}{
	cfgApplicationName:      {},
	cfgApplicationVersion:   {},
	cfgApplicationBuildTime: {},

	cfgPeers: {},

	cmdHelp:    {},
	cmdVersion: {},
}

func (empty) Read([]byte) (int, error) { return 0, io.EOF }

func settings() *viper.Viper {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvPrefix(Prefix)
	v.SetConfigType("yaml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// flags setup:
	flags := pflag.NewFlagSet("commandline", pflag.ExitOnError)
	flags.SetOutput(os.Stdout)
	flags.SortFlags = false

	flags.Bool("pprof", false, "enable pprof")
	flags.Bool("metrics", false, "enable prometheus")

	help := flags.BoolP(cmdHelp, "h", false, "show help")
	version := flags.BoolP(cmdVersion, "v", false, "show version")

	flags.String("key", generated, `"`+generated+`" to generate key, path to private key file, hex string or wif`)

	flags.Bool("verbose", false, "debug gRPC connections")
	flags.Duration("request_timeout", defaultRequestTimeout, "gRPC request timeout")
	flags.Duration("connect_timeout", defaultConnectTimeout, "gRPC connect timeout")
	flags.Duration("rebalance_timer", defaultRebalanceTimer, "gRPC connection rebalance timer")

	ttl := flags.DurationP("conn_ttl", "t", defaultTTL, "gRPC connection time to live")

	flags.String("listen_address", "0.0.0.0:8082", "HTTP Gateway listen address")
	peers := flags.StringArrayP("peers", "p", nil, "NeoFS nodes")

	// set prefers:
	v.Set(cfgApplicationName, "neofs-http-gw")
	v.Set(cfgApplicationVersion, Version)

	// set defaults:

	// logger:
	v.SetDefault(cfgLoggerLevel, "debug")
	v.SetDefault(cfgLoggerFormat, "console")
	v.SetDefault(cfgLoggerTraceLevel, "panic")
	v.SetDefault(cfgLoggerNoCaller, false)
	v.SetDefault(cfgLoggerNoDisclaimer, true)
	v.SetDefault(cfgLoggerSamplingInitial, 1000)
	v.SetDefault(cfgLoggerSamplingThereafter, 1000)

	// keepalive:
	// If set below 10s, a minimum value of 10s will be used instead.
	v.SetDefault("keepalive.time", defaultKeepaliveTime)
	v.SetDefault("keepalive.timeout", defaultKeepaliveTimeout)
	v.SetDefault("keepalive.permit_without_stream", true)

	// web-server:
	v.SetDefault("web.read_buffer_size", 4096)
	v.SetDefault("web.write_buffer_size", 4096)
	v.SetDefault("web.read_timeout", time.Second*15)
	v.SetDefault("web.write_timeout", time.Minute)
	v.SetDefault("web.connection_per_host", 10)

	if err := v.BindPFlags(flags); err != nil {
		panic(err)
	}

	if err := v.ReadConfig(devNull); err != nil {
		panic(err)
	}

	if err := flags.Parse(os.Args); err != nil {
		panic(err)
	}

	switch {
	case help != nil && *help:
		fmt.Printf("NeoFS HTTP Gateway %s (%s)\n", Version, Build)
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
		fmt.Printf("%s_%s_[N]_WEIGHT = 0..1 (float)\n", Prefix, strings.ToUpper(cfgPeers))

		os.Exit(0)
	case version != nil && *version:
		fmt.Printf("NeoFS HTTP Gateway %s (%s)\n", Version, Build)
		os.Exit(0)
	case ttl != nil && ttl.Minutes() < minimumTTLInMinutes:
		fmt.Printf("connection ttl should not be less than %s", defaultTTL)
	}

	if peers != nil && len(*peers) > 0 {
		for i := range *peers {
			v.SetDefault(cfgPeers+"."+strconv.Itoa(i)+".address", (*peers)[i])
			v.SetDefault(cfgPeers+"."+strconv.Itoa(i)+".weight", 1)
		}
	}

	return v
}
