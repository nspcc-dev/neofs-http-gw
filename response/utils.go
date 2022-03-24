package response

import "github.com/valyala/fasthttp"

// Error add new line to msg and invoke r.Error.
func Error(r *fasthttp.RequestCtx, msg string, code int) {
	r.Error(msg+"\n", code)
}
