package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	crypto "github.com/nspcc-dev/neofs-crypto"
	"github.com/nspcc-dev/neofs-proto/refs"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type empty int

const (
	devNull  = empty(0)
	generate = "gen"
)

func (empty) Read([]byte) (int, error) { return 0, io.EOF }

func fetchKey(l *zap.Logger, v *viper.Viper) *ecdsa.PrivateKey {
	switch val := v.GetString("key"); val {
	case generate:
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			l.Fatal("could not generate private key", zap.Error(err))
		}

		id, err := refs.NewOwnerID(&key.PublicKey)
		l.Info("generate new key",
			zap.Stringer("key", id),
			zap.Error(err))

		return key

	default:
		key, err := crypto.LoadPrivateKey(val)
		if err != nil {
			l.Fatal("could not load private key",
				zap.String("key", v.GetString("key")),
				zap.Error(err))
		}

		return key
	}
}

func settings() *viper.Viper {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvPrefix("GW")
	v.SetConfigType("yaml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// flags setup:
	flags := pflag.NewFlagSet("comandline", pflag.ExitOnError)
	flags.SortFlags = false

	help := flags.BoolP("help", "h", false, "show help")
	version := flags.BoolP("version", "v", false, "show version")

	flags.String("key", "gen", `"gen" to generate key, path to private key file, hex string or wif`)

	flags.Bool("verbose", false, "debug gRPC connections")
	flags.Duration("request_timeout", time.Second*5, "gRPC request timeout")
	flags.Duration("connect_timeout", time.Second*30, "gRPC connect timeout")

	flags.String("listen_address", "0.0.0.0:8082", "HTTP Gateway listen address")
	flags.String("neofs_address", "0.0.0.0:8080", "NeoFS Node address for proxying requests")

	// set prefers:
	v.Set("app.name", "neofs-gw")
	v.Set("app.version", Version)

	// set defaults:

	// logger:
	v.SetDefault("logger.level", "debug")
	v.SetDefault("logger.format", "console")
	v.SetDefault("logger.trace_level", "fatal")
	v.SetDefault("logger.no_disclaimer", true)
	v.SetDefault("logger.sampling.initial", 1000)
	v.SetDefault("logger.sampling.thereafter", 1000)

	// keepalive:
	v.SetDefault("keepalive.timeout", time.Second*10)
	v.SetDefault("keepalive.time", time.Millisecond*100)
	v.SetDefault("keepalive.permit_without_stream", true)

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
		os.Exit(0)
	case version != nil && *version:
		fmt.Printf("NeoFS HTTP Gateway %s (%s)\n", Version, Build)
		os.Exit(0)
	}

	return v
}
