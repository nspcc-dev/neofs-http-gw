module github.com/nspcc-dev/neofs-http-gw

go 1.16

require (
	github.com/fasthttp/router v1.3.5
	github.com/mr-tron/base58 v1.1.3 // indirect
	github.com/nspcc-dev/neofs-api-go v1.27.0
	github.com/nspcc-dev/neofs-crypto v0.3.0
	github.com/nspcc-dev/neofs-sdk-go v0.0.0-20210615074944-86a9aa92599b
	github.com/prometheus/client_golang v1.9.0
	github.com/prometheus/common v0.15.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.7.0
	github.com/valyala/fasthttp v1.22.0
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/zap v1.16.0
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2 // indirect
	golang.org/x/lint v0.0.0-20191125180803-fdd1cda4f05f // indirect
	golang.org/x/tools v0.0.0-20200123022218-593de606220b // indirect
	google.golang.org/grpc v1.36.1
)

replace github.com/valyala/fasthttp => github.com/nspcc-dev/fasthttp v1.19.1-0.20210428122823-ab82e78c7994
