package main

import (
	"bytes"
	"encoding/base64"

	"github.com/nspcc-dev/neofs-api-go/pkg/token"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
)

const bearerToken = "Bearer"

// BearerToken usage:
//
// if tkn, err = BearerToken(c); err != nil && tkn == nil {
// 	log.Error("could not fetch bearer token", zap.Error(err))
// 	c.Error("could not fetch bearer token", fasthttp.StatusBadRequest)
// 	return
// }
var _ = BearerToken

func headerAuth(h *fasthttp.RequestHeader) (*token.BearerToken, error) {
	auth := h.Peek(fasthttp.HeaderAuthorization)
	if auth == nil || !bytes.Contains(auth, []byte(bearerToken)) {
		return nil, nil
	}

	auth = bytes.ReplaceAll(auth, []byte(bearerToken+" "), nil)

	data, err := base64.StdEncoding.DecodeString(string(auth))
	if err != nil {
		return nil, errors.Wrap(err, "could not fetch marshaled from base64")
	}

	tkn := new(token.BearerToken)
	if err = tkn.Unmarshal(data); err != nil {
		return nil, errors.Wrap(err, "could unmarshal bearer token")
	}

	return tkn, nil
}

func cookieAuth(h *fasthttp.RequestHeader) (*token.BearerToken, error) {
	auth := h.Cookie(bearerToken)
	if auth == nil {
		return nil, nil
	}

	data, err := base64.StdEncoding.DecodeString(string(auth))
	if err != nil {
		return nil, errors.Wrap(err, "could not fetch marshaled from base64")
	}

	tkn := new(token.BearerToken)
	if err = tkn.Unmarshal(data); err != nil {
		return nil, errors.Wrap(err, "could unmarshal bearer token")
	}

	return tkn, nil
}

func BearerToken(ctx *fasthttp.RequestCtx) (*token.BearerToken, error) {
	// ignore empty value
	if ctx == nil {
		return nil, nil
	}

	if tkn, err := headerAuth(&ctx.Request.Header); err != nil {
		return nil, err
	} else if tkn != nil {
		return tkn, nil
	}

	return cookieAuth(&ctx.Request.Header)
}
