package handlers

import (
	"fmt"

	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neofs-http-gw/response"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

// API is a REST v1 request handler.
type API struct {
	log              *zap.Logger
	pool             *pool.Pool
	defaultTimestamp bool
}

// PrmAPI groups parameters to init rest API.
type PrmAPI struct {
	Logger           *zap.Logger
	Pool             *pool.Pool
	DefaultTimestamp bool
}

// New creates a new API using specified logger, connection pool and other parameters.
func New(prm *PrmAPI) *API {
	return &API{
		log:              prm.Logger,
		pool:             prm.Pool,
		defaultTimestamp: prm.DefaultTimestamp,
	}
}

func (a *API) logAndSendError(c *fasthttp.RequestCtx, msg string, err error, status int) {
	a.log.Error(msg, zap.Error(err))
	response.Error(c, msg+": "+err.Error(), status)
}

func fetchBearerOwner(header *fasthttp.RequestHeader) (*keys.PublicKey, error) {
	ownerKey := header.Peek(XNeofsBearerOwnerKey)
	if ownerKey == nil {
		return nil, fmt.Errorf("missing header %s", XNeofsBearerOwnerKey)
	}

	return keys.NewPublicKeyFromString(string(ownerKey))
}
