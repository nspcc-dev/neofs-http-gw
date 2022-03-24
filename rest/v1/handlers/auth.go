package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/nspcc-dev/neofs-api-go/v2/acl"
	"github.com/nspcc-dev/neofs-http-gw/rest/v1/model"
	"github.com/nspcc-dev/neofs-sdk-go/client"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/valyala/fasthttp"
)

const defaultBearerExpDuration = 100 // in epoch

// AuthHandler handler that forms bearer token to sign.
func (a *API) AuthHandler(c *fasthttp.RequestCtx) {
	ctx, cancel := context.WithCancel(c)
	defer cancel()

	var bearer model.Bearer
	if err := json.NewDecoder(c.RequestBodyStream()).Decode(&bearer); err != nil {
		a.logAndSendError(c, "couldn't decode bearer token", err, fasthttp.StatusBadRequest)
		return
	}
	btoken, err := bearer.ToNative()
	if err != nil {
		a.logAndSendError(c, "couldn't transform bearer token to native", err, fasthttp.StatusBadRequest)
		return
	}
	btoken.SetOwner(a.pool.OwnerID())

	lifetime, err := getBearerLifetime(ctx, &c.Request.Header, a.pool)
	if err != nil {
		a.logAndSendError(c, "couldn't lifetime", err, fasthttp.StatusInternalServerError)
		return
	}
	btoken.ToV2().GetBody().SetLifetime(lifetime)

	binaryBearer, err := btoken.ToV2().GetBody().StableMarshal(nil)
	if err != nil {
		a.logAndSendError(c, "couldn't marshal bearer token", err, fasthttp.StatusInternalServerError)
		return
	}

	base64Bearer := base64.StdEncoding.EncodeToString(binaryBearer)

	c.SetContentType("application/json")
	c.Response.SetBodyString(base64Bearer)
}

func getCurrentEpoch(ctx context.Context, p *pool.Pool) (uint64, error) {
	conn, _, err := p.Connection()
	if err != nil {
		return 0, fmt.Errorf("couldn't get connection: %w", err)
	}

	netInfoRes, err := conn.NetworkInfo(ctx, client.PrmNetworkInfo{})
	if err != nil {
		return 0, fmt.Errorf("couldn't get netwokr info: %w", err)
	}

	return netInfoRes.Info().CurrentEpoch(), nil
}

func getBearerLifetime(ctx context.Context, header *fasthttp.RequestHeader, p *pool.Pool) (*acl.TokenLifetime, error) {
	currEpoch, err := getCurrentEpoch(ctx, p)
	if err != nil {
		return nil, err
	}

	var lifetimeDuration uint64 = defaultBearerExpDuration
	expDuration := header.Peek(XNeofsBearerLifetime)
	if expDuration != nil {
		lifetimeDuration, err = strconv.ParseUint(string(expDuration), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("coudln't parse token lifetime duration")
		}
	}

	lifetime := new(acl.TokenLifetime)
	lifetime.SetIat(currEpoch)
	lifetime.SetExp(currEpoch + lifetimeDuration)

	return lifetime, nil
}
