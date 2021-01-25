package main

import (
	"bytes"
	"encoding/base64"

	"github.com/pkg/errors"

	"github.com/nspcc-dev/neofs-api-go/pkg/token"
	"github.com/valyala/fasthttp"
)

type fromHandler = func(h *fasthttp.RequestHeader) []byte

const bearerToken = "Bearer"

// BearerToken usage:
//
// if tkn, err = BearerToken(c); err != nil && tkn == nil {
// 	log.Error("could not fetch bearer token", zap.Error(err))
// 	c.Error("could not fetch bearer token", fasthttp.StatusBadRequest)
// 	return
// }
var _ = BearerToken

func fromHeader(h *fasthttp.RequestHeader) []byte {
	auth := h.Peek(fasthttp.HeaderAuthorization)
	if auth == nil || !bytes.HasPrefix(auth, []byte(bearerToken)) {
		return nil
	}

	if auth = bytes.TrimPrefix(auth, []byte(bearerToken+" ")); len(auth) == 0 {
		return nil
	}

	return auth
}

func fromCookie(h *fasthttp.RequestHeader) []byte {
	auth := h.Cookie(bearerToken)
	if len(auth) == 0 {
		return nil
	}

	return auth
}

func BearerToken(ctx *fasthttp.RequestCtx) (*token.BearerToken, error) {
	// ignore empty value
	if ctx == nil {
		panic(nil)
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
