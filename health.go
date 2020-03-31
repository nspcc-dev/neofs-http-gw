package main

import (
	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
	"go.uber.org/atomic"
)

const (
	healthyState       = "NeoFS HTTP Gateway is "
	defaultContentType = "text/plain; charset=utf-8"
)

func attachHealthy(r *router.Router, e *atomic.Error) {
	r.GET("/-/ready/", func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString(healthyState + "ready")
	})

	r.GET("/-/healthy/", func(c *fasthttp.RequestCtx) {
		code := fasthttp.StatusOK
		msg := "healthy"

		if err := e.Load(); err != nil {
			msg = "unhealthy: " + err.Error()
			code = fasthttp.StatusBadRequest
		}

		c.Response.Reset()
		c.SetStatusCode(code)
		c.SetContentType(defaultContentType)
		c.SetBodyString(healthyState + msg)
	})
}
