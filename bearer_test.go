package main

import (
	"encoding/base64"
	"testing"

	"github.com/nspcc-dev/neofs-api-go/pkg/owner"

	"github.com/nspcc-dev/neofs-api-go/pkg/token"

	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func makeTestCookie(value []byte) *fasthttp.RequestHeader {
	header := new(fasthttp.RequestHeader)
	header.SetCookie(bearerToken, string(value))
	return header
}

func makeTestHeader(value []byte) *fasthttp.RequestHeader {
	header := new(fasthttp.RequestHeader)
	if value != nil {
		header.Set(fasthttp.HeaderAuthorization, bearerToken+" "+string(value))
	}
	return header
}

func Test_fromCookie(t *testing.T) {
	cases := []struct {
		name   string
		actual []byte
		expect []byte
	}{
		{name: "empty"},
		{name: "normal", actual: []byte("TOKEN"), expect: []byte("TOKEN")},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expect, fromCookie(makeTestCookie(tt.actual)))
		})
	}
}

func Test_fromHeader(t *testing.T) {
	cases := []struct {
		name   string
		actual []byte
		expect []byte
	}{
		{name: "empty"},
		{name: "normal", actual: []byte("TOKEN"), expect: []byte("TOKEN")},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expect, fromHeader(makeTestHeader(tt.actual)))
		})
	}
}

func TestBearerToken(t *testing.T) {
	uid := owner.NewID()

	tkn := new(token.BearerToken)
	tkn.SetOwner(uid)

	data, err := tkn.Marshal()

	require.NoError(t, err)

	t64 := base64.StdEncoding.EncodeToString(data)
	require.NotEmpty(t, t64)

	cases := []struct {
		name string

		cookie string
		header string

		error  string
		expect *token.BearerToken
	}{
		{name: "empty"},

		{name: "bad base64 header", header: "WRONG BASE64", error: "could not fetch marshaled from base64"},
		{name: "bad base64 cookie", cookie: "WRONG BASE64", error: "could not fetch marshaled from base64"},

		{name: "header token unmarshal error", header: "dGVzdAo=", error: "could not unmarshal bearer token"},
		{name: "cookie token unmarshal error", cookie: "dGVzdAo=", error: "could not unmarshal bearer token"},

		{
			name:   "bad header and cookie",
			header: "WRONG BASE64",
			cookie: "dGVzdAo=",
			error:  "could not unmarshal bearer token",
		},

		{
			name:   "bad header, but good cookie",
			header: "dGVzdAo=",
			cookie: t64,
			expect: tkn,
		},

		{name: "ok for header", header: t64, expect: tkn},
		{name: "ok for cookie", cookie: t64, expect: tkn},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := makeTestRequest(tt.cookie, tt.header)
			actual, err := BearerToken(ctx)

			if tt.error == "" {
				require.NoError(t, err)
				require.Equal(t, tt.expect, actual)

				return
			}

			require.Contains(t, err.Error(), tt.error)
		})
	}
}

func makeTestRequest(cookie, header string) *fasthttp.RequestCtx {
	ctx := new(fasthttp.RequestCtx)

	if cookie != "" {
		ctx.Request.Header.SetCookie(bearerToken, cookie)
	}

	if header != "" {
		ctx.Request.Header.Set(fasthttp.HeaderAuthorization, bearerToken+" "+header)
	}
	return ctx
}
