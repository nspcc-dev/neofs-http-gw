package response

import "github.com/valyala/fasthttp"

func Error(r *fasthttp.RequestCtx, msg string, code int) {
	r.Error(msg+"\n", code)
}
