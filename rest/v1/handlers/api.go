package handlers

import (
	"encoding/json"
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
	key              *keys.PrivateKey
	defaultTimestamp bool
}

// PrmAPI groups parameters to init rest API.
type PrmAPI struct {
	Logger           *zap.Logger
	Pool             *pool.Pool
	Key              *keys.PrivateKey
	DefaultTimestamp bool
}

// New creates a new API using specified logger, connection pool and other parameters.
func New(prm *PrmAPI) *API {
	return &API{
		log:              prm.Logger,
		pool:             prm.Pool,
		key:              prm.Key,
		defaultTimestamp: prm.DefaultTimestamp,
	}
}

func (a *API) encodeAndSend(c *fasthttp.RequestCtx, data interface{}) {
	c.Response.SetStatusCode(fasthttp.StatusOK)
	c.Response.Header.SetContentType("application/json")

	enc := json.NewEncoder(c)
	enc.SetIndent("", "\t")
	if err := enc.Encode(data); err != nil {
		a.logAndSendError(c, "could not encode response", err, fasthttp.StatusBadRequest)
	}
}

func (a *API) logAndSendError(c *fasthttp.RequestCtx, msg string, err error, status int) {
	a.log.Error(msg, zap.Error(err))
	response.Error(c, msg+": "+err.Error(), status)
}

func fetchBearerOwner(header *fasthttp.RequestHeader) (*keys.PublicKey, error) {
	ownerKey := header.Peek(XNeofsTokenSignatureKey)
	if ownerKey == nil {
		return nil, fmt.Errorf("missing header %s", XNeofsTokenSignatureKey)
	}

	return keys.NewPublicKeyFromString(string(ownerKey))
}
