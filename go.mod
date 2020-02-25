module github.com/nspcc-dev/neofs-gw

go 1.13

require (
	github.com/labstack/echo/v4 v4.1.14 // v4.1.11 => v4.1.14
	github.com/nspcc-dev/neofs-api v0.0.0-00000000000000-000000000000
	github.com/nspcc-dev/neofs-crypto v0.2.3
	github.com/pkg/errors v0.9.1 // v0.8.1 => v0.9.1
	github.com/prometheus/client_golang v1.4.1 // v1.2.1 => v1.4.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.6.2 // v1.6.1 => v1.6.2
	go.uber.org/atomic v1.5.0
	go.uber.org/zap v1.13.0
	google.golang.org/grpc v1.27.1
)

// Temporary, before we move repo to github:
// replace github.com/nspcc-dev/neofs-proto => ../neofs-proto

// For debug reasons
replace (
	github.com/nspcc-dev/neofs-api => ../neofs-api
	google.golang.org/grpc => ../grpc-go
)
