module github.com/nspcc-dev/neofs-http-gate

go 1.16

require (
    github.com/fasthttp/router v0.6.1
    github.com/nspcc-dev/neofs-api v0.0.0-00000000000000-000000000000
    github.com/nspcc-dev/neofs-crypto v0.2.3
    github.com/prometheus/client_golang v1.4.1 // v1.2.1 => v1.4.1
    github.com/prometheus/common v0.9.1
    github.com/spf13/pflag v1.0.5
    github.com/spf13/viper v1.6.2 // v1.6.1 => v1.6.2
    github.com/valyala/fasthttp v1.9.0
    go.uber.org/atomic v1.6.0
    go.uber.org/zap v1.14.0
    golang.org/x/crypto v0.0.0-20191227163750-53104e6ec876 // indirect
    golang.org/x/net v0.0.0-20191209160850-c0dbc17a3553 // indirect
    golang.org/x/text v0.3.2 // indirect
    google.golang.org/grpc v1.27.1
)

// For debug reasons
replace (
    github.com/nspcc-dev/neofs-api => ../neofs-api
    google.golang.org/grpc => ../grpc-go
)
