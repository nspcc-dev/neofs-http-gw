package main

import (
	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
)

type stater func() error

const (
	healthyState       = "NeoFS HTTP Gateway is "
	defaultContentType = "text/plain; charset=utf-8"
)

func attachHealthy(r *router.Router, e stater) {
	r.GET("/-/ready/", func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString(healthyState + "ready")
	})
	r.GET("/-/healthy/", func(c *fasthttp.RequestCtx) {
		code := fasthttp.StatusOK
		msg := "healthy"

		if err := e(); err != nil {
			msg = "unhealthy: " + err.Error()
			code = fasthttp.StatusBadRequest
		}
		c.Response.Reset()
		c.SetStatusCode(code)
		c.SetContentType(defaultContentType)
		c.SetBodyString(healthyState + msg)
	})
}
