package utils

import (
	"github.com/nspcc-dev/neofs-http-gw/resolver"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/nspcc-dev/neofs-sdk-go/user"
	"go.uber.org/zap"
)

type AppParams struct {
	Logger   *zap.Logger
	Pool     *pool.Pool
	Owner    *user.ID
	Resolver *resolver.ContainerResolver
}
