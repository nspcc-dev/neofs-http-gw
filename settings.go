package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nspcc-dev/neofs-api/refs"
	crypto "github.com/nspcc-dev/neofs-crypto"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type empty int

const (
	devNull   = empty(0)
	generated = "generated"
)

func (empty) Read([]byte) (int, error) { return 0, io.EOF }

func fetchKey(l *zap.Logger, v *viper.Viper) *ecdsa.PrivateKey {
	switch val := v.GetString("key"); val {
	case generated:
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
	flags := pflag.NewFlagSet("commandline", pflag.ExitOnError)
	flags.SortFlags = false

	flags.Bool("pprof", false, "enable pprof")
	flags.Bool("metrics", false, "enable prometheus")

	help := flags.BoolP("help", "h", false, "show help")
	version := flags.BoolP("version", "v", false, "show version")

	flags.String("key", generated, `"`+generated+`" to generate key, path to private key file, hex string or wif`)

	flags.Bool("verbose", false, "debug gRPC connections")
	flags.Duration("request_timeout", time.Second*5, "gRPC request timeout")
	flags.Duration("connect_timeout", time.Second*30, "gRPC connect timeout")

	flags.String("listen_address", "0.0.0.0:8082", "HTTP Gateway listen address")
	peers := flags.StringArrayP("peers", "p", nil, "NeoFS nodes")

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

	if peers != nil && len(*peers) > 0 {
		for i := range *peers {
			v.SetDefault("peers."+strconv.Itoa(i)+".address", (*peers)[i])
			v.SetDefault("peers."+strconv.Itoa(i)+".weight", 1)
		}
	}

	return v
}