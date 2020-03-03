package main

import (
	"net/http/pprof"
	rtp "runtime/pprof"

	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

func attachProfiler(r *router.Router) {
	r.GET("/debug/pprof/", pprofHandler())
	r.GET("/debug/pprof/:name", pprofHandler())
}

func pprofHandler() fasthttp.RequestHandler {
	items := rtp.Profiles()

	profiles := map[string]fasthttp.RequestHandler{
		"":        fasthttpadaptor.NewFastHTTPHandlerFunc(pprof.Index),
		"cmdline": fasthttpadaptor.NewFastHTTPHandlerFunc(pprof.Cmdline),
		"profile": fasthttpadaptor.NewFastHTTPHandlerFunc(pprof.Profile),
		"symbol":  fasthttpadaptor.NewFastHTTPHandlerFunc(pprof.Symbol),
		"trace":   fasthttpadaptor.NewFastHTTPHandlerFunc(pprof.Trace),
	}

	for i := range items {
		name := items[i].Name()
		profiles[name] = fasthttpadaptor.NewFastHTTPHandler(pprof.Handler(name))
	}

	return func(ctx *fasthttp.RequestCtx) {
		name, _ := ctx.UserValue("name").(string)

		if handler, ok := profiles[name]; ok {
			handler(ctx)
			return
		}

		ctx.Error("Not found", fasthttp.StatusNotFound)
	}
}
