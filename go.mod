module github.com/nspcc-dev/neofs-gw

go 1.13

require (
	github.com/labstack/echo/v4 v4.1.11
	github.com/nspcc-dev/hrw v1.0.9 // indirect
	github.com/nspcc-dev/neofs-proto v0.1.0
	github.com/pkg/errors v0.8.1
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.5.0
	go.uber.org/zap v1.11.0
	golang.org/x/text v0.3.2 // indirect
	google.golang.org/grpc v1.24.0
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect

)

// Temporary, before we move repo to github:
replace github.com/nspcc-dev/neofs-proto v0.0.0-20191101093315-0d5a92f60568 => bitbucket.org/nspcc-dev/neofs-proto v0.0.0-20191101093315-0d5a92f60568
