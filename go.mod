module github.com/nspcc-dev/neofs-gw

go 1.13

require (
	github.com/labstack/echo/v4 v4.1.11
	github.com/nspcc-dev/neofs-crypto v0.2.3
	github.com/nspcc-dev/neofs-proto v0.2.11
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.6.1
	go.uber.org/zap v1.13.0
	google.golang.org/grpc v1.25.1
)

// Temporary, before we move repo to github:
// replace github.com/nspcc-dev/neofs-proto => ../neofs-proto
