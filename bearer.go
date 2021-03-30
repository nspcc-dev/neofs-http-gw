package main

import (
	"bytes"
	"context"
	"encoding/base64"

	"github.com/nspcc-dev/neofs-api-go/pkg/token"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
)

type fromHandler = func(h *fasthttp.RequestHeader) []byte

const (
	bearerTokenHdr = "Bearer"
	bearerTokenKey = "__context_bearer_token_key"
)

// BearerToken usage:
//
// if err = storeBearerToken(ctx); err != nil {
// 	log.Error("could not fetch bearer token", zap.Error(err))
// 	c.Error("could not fetch bearer token", fasthttp.StatusBadRequest)
// 	return
// }

func fromHeader(h *fasthttp.RequestHeader) []byte {
	auth := h.Peek(fasthttp.HeaderAuthorization)
	if auth == nil || !bytes.HasPrefix(auth, []byte(bearerTokenHdr)) {
		return nil
	}

	if auth = bytes.TrimPrefix(auth, []byte(bearerTokenHdr+" ")); len(auth) == 0 {
		return nil
	}

	return auth
}

func fromCookie(h *fasthttp.RequestHeader) []byte {
	auth := h.Cookie(bearerTokenHdr)
	if len(auth) == 0 {
		return nil
	}

	return auth
}

func storeBearerToken(ctx *fasthttp.RequestCtx) error {
	tkn, err := fetchBearerToken(ctx)
	if err != nil {
		return err
	}
	// This is an analog of context.WithValue.
	ctx.SetUserValue(bearerTokenKey, tkn)
	return nil
}

func fetchBearerToken(ctx *fasthttp.RequestCtx) (*token.BearerToken, error) {
	// ignore empty value
	if ctx == nil {
		return nil, nil
	}
	var (
		lastErr error

		buf []byte
		tkn = new(token.BearerToken)
	)
	for _, parse := range []fromHandler{fromHeader, fromCookie} {
		if buf = parse(&ctx.Request.Header); buf == nil {
			continue
		} else if data, err := base64.StdEncoding.DecodeString(string(buf)); err != nil {
			lastErr = errors.Wrap(err, "could not fetch marshaled from base64")
			continue
		} else if err = tkn.Unmarshal(data); err != nil {
			lastErr = errors.Wrap(err, "could not unmarshal bearer token")
			continue
		} else if tkn == nil {
			continue
		}

		return tkn, nil
	}

	return nil, lastErr
}

func loadBearerToken(ctx context.Context) (*token.BearerToken, error) {
	if tkn, ok := ctx.Value(bearerTokenKey).(*token.BearerToken); ok && tkn != nil {
		return tkn, nil
	}
	return nil, errors.New("found empty bearer token")
}
