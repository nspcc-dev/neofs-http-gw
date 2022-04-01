package handlers

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neofs-http-gw/rest/v1/model"
	"github.com/nspcc-dev/neofs-sdk-go/client"
	"github.com/nspcc-dev/neofs-sdk-go/owner"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/valyala/fasthttp"
)

const defaultTokenExpDuration = 100 // in epoch

// AuthHandler handler that forms bearer token to sign.
func (a *API) AuthHandler(c *fasthttp.RequestCtx) {
	ctx, cancel := context.WithCancel(c)
	defer cancel()

	var bearer model.Bearer
	if err := json.NewDecoder(c.RequestBodyStream()).Decode(&bearer); err != nil {
		a.logAndSendError(c, "couldn't decode bearer token", err, fasthttp.StatusBadRequest)
		return
	}

	scope, err := getTokenScope(&c.Request.Header)
	if err == nil && scope == UnknownScope {
		err = fmt.Errorf("invalid scope")
	}
	if err != nil {
		a.logAndSendError(c, "couldn't parse token scope", err, fasthttp.StatusBadRequest)
		return
	}

	if scope == ObjectScope {
		base64Bearer, err := prepareObjectToken(ctx, &c.Request.Header, a.pool, &bearer)
		if err != nil {
			a.logAndSendError(c, "prepare object token", err, fasthttp.StatusBadRequest)
			return
		}

		c.SetContentType("application/json")
		c.Response.SetBodyString(base64Bearer)
	} else {
		tokens, err := prepareContainerTokens(ctx, &c.Request.Header, a.pool, &bearer, a.key.PublicKey())
		if err != nil {
			a.logAndSendError(c, "prepare container tokens", err, fasthttp.StatusBadRequest)
			return
		}

		c.Response.Header.SetContentType("application/json")
		enc := json.NewEncoder(c)
		enc.SetIndent("", "\t")
		if err = enc.Encode(tokens); err != nil {
			a.logAndSendError(c, "could not encode response", err, fasthttp.StatusBadRequest)
			return
		}
	}
}

func prepareObjectToken(ctx context.Context, header *fasthttp.RequestHeader, pool *pool.Pool, bearer *model.Bearer) (string, error) {
	btoken, err := bearer.ToNativeObjectToken()
	if err != nil {
		return "", fmt.Errorf("couldn't transform token to native: %w", err)
	}
	btoken.SetOwner(pool.OwnerID())

	iat, exp, err := getTokenLifetime(ctx, header, pool)
	if err != nil {
		return "", fmt.Errorf("couldn't get lifetime: %w", err)
	}
	btoken.SetLifetime(exp, 0, iat)

	binaryBearer, err := btoken.ToV2().GetBody().StableMarshal(nil)
	if err != nil {
		return "", fmt.Errorf("couldn't marshal bearer token: %w", err)
	}

	return base64.StdEncoding.EncodeToString(binaryBearer), nil
}

func prepareContainerTokens(ctx context.Context, header *fasthttp.RequestHeader, pool *pool.Pool, bearer *model.Bearer, key *keys.PublicKey) (*model.ContainerTokenResponse, error) {
	iat, exp, err := getTokenLifetime(ctx, header, pool)
	if err != nil {
		return nil, fmt.Errorf("couldn't get lifetime: %w", err)
	}

	ownerKey, err := fetchBearerOwner(header)
	if err != nil {
		return nil, err
	}

	var resp model.ContainerTokenResponse
	resp.Tokens = make([]model.ContainerToken, len(bearer.ContainerRules))

	for i, rule := range bearer.ContainerRules {
		stoken, err := rule.ToNativeContainerToken()
		if err != nil {
			return nil, fmt.Errorf("couldn't transform rule to native session token: %w", err)
		}

		uid, err := uuid.New().MarshalBinary()
		if err != nil {
			return nil, err
		}
		stoken.SetID(uid)

		stoken.SetOwnerID(owner.NewIDFromPublicKey((*ecdsa.PublicKey)(ownerKey)))

		stoken.SetIat(iat)
		stoken.SetExp(exp)
		stoken.SetSessionKey(key.Bytes())

		binaryToken, err := stoken.ToV2().GetBody().StableMarshal(nil)
		if err != nil {
			return nil, fmt.Errorf("couldn't marshal session token: %w", err)
		}

		resp.Tokens[i] = model.ContainerToken{
			Verb:  rule.Verb,
			Token: base64.StdEncoding.EncodeToString(binaryToken),
		}
	}

	return &resp, nil
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

func getTokenLifetime(ctx context.Context, header *fasthttp.RequestHeader, p *pool.Pool) (uint64, uint64, error) {
	currEpoch, err := getCurrentEpoch(ctx, p)
	if err != nil {
		return 0, 0, err
	}

	var lifetimeDuration uint64 = defaultTokenExpDuration
	expDuration := header.Peek(XNeofsTokenLifetime)
	if expDuration != nil {
		lifetimeDuration, err = strconv.ParseUint(string(expDuration), 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("coudln't parse token lifetime duration")
		}
	}

	return currEpoch, currEpoch + lifetimeDuration, nil
}

func getTokenScope(header *fasthttp.RequestHeader) (TokenScope, error) {
	scopeHeaderValue := header.Peek(XNeofsTokenScope)
	if scopeHeaderValue == nil {
		return UnknownScope, fmt.Errorf("missed %s header", XNeofsTokenScope)
	}

	var scope TokenScope
	scope.Parse(string(scopeHeaderValue))
	return scope, nil
}
