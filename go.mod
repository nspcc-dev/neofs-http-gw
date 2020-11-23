module github.com/nspcc-dev/neofs-http-gate

go 1.13

require (
	github.com/fasthttp/router v1.1.6
	github.com/nspcc-dev/cdn-neofs-sdk v0.0.0
	github.com/nspcc-dev/neofs-api-go v1.20.2
	github.com/nspcc-dev/neofs-crypto v0.3.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.6.0
	github.com/prometheus/common v0.10.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.0
	github.com/valyala/fasthttp v1.14.0
	go.uber.org/zap v1.16.0
	google.golang.org/grpc v1.33.1
)

replace github.com/nspcc-dev/cdn-neofs-sdk => ../sdk
