package main

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"
	"github.com/valyala/fasthttp"
)

func metricsHandler(reg prometheus.Gatherer, opts promhttp.HandlerOpts) fasthttp.RequestHandler {
	var (
		inFlightSem chan struct{}
		errCnt      = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "promhttp_metric_handler_errors_total",
				Help: "Total number of internal errors encountered by the promhttp metric handler.",
			},
			[]string{"cause"},
		)
	)

	if opts.MaxRequestsInFlight > 0 {
		inFlightSem = make(chan struct{}, opts.MaxRequestsInFlight)
	}
	if opts.Registry != nil {
		// Initialize all possibilites that can occur below.
		errCnt.WithLabelValues("gathering")
		errCnt.WithLabelValues("encoding")
		if err := opts.Registry.Register(errCnt); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				errCnt = are.ExistingCollector.(*prometheus.CounterVec)
			} else {
				panic(err)
			}
		}
	}

	h := fasthttp.RequestHandler(func(c *fasthttp.RequestCtx) {
		if inFlightSem != nil {
			select {
			case inFlightSem <- struct{}{}: // All good, carry on.
				defer func() { <-inFlightSem }()
			default:

				c.Error(fmt.Sprintf(
					"Limit of concurrent requests reached (%d), try again later.", opts.MaxRequestsInFlight,
				), fasthttp.StatusServiceUnavailable)
				return
			}
		}
		mfs, err := reg.Gather()
		if err != nil {
			if opts.ErrorLog != nil {
				panic("error gathering metrics:" + err.Error())
			}

			errCnt.WithLabelValues("gathering").Inc()
			switch opts.ErrorHandling {
			case promhttp.PanicOnError:
				panic(err)
			case promhttp.ContinueOnError:
				if len(mfs) == 0 {
					// Still report the error if no metrics have been gathered.
					c.Error(err.Error(), fasthttp.StatusServiceUnavailable)
					return
				}
			case promhttp.HTTPErrorOnError:
				c.Error(err.Error(), fasthttp.StatusServiceUnavailable)
				return
			}
		}

		contentType := expfmt.FmtText
		c.SetContentType(string(contentType))
		enc := expfmt.NewEncoder(c, contentType)

		var lastErr error

		// handleError handles the error according to opts.ErrorHandling
		// and returns true if we have to abort after the handling.
		handleError := func(err error) bool {
			if err == nil {
				return false
			}
			lastErr = err
			if opts.ErrorLog != nil {
				opts.ErrorLog.Println("error encoding and sending metric family:", err)
			}
			errCnt.WithLabelValues("encoding").Inc()
			switch opts.ErrorHandling {
			case promhttp.PanicOnError:
				panic(err)
			case promhttp.HTTPErrorOnError:
				c.Error(err.Error(), fasthttp.StatusServiceUnavailable)
				return true
			}
			// Do nothing in all other cases, including ContinueOnError.
			return false
		}

		for _, mf := range mfs {
			if handleError(enc.Encode(mf)) {
				return
			}
		}
		if closer, ok := enc.(expfmt.Closer); ok {
			// This in particular takes care of the final "# EOF\n" line for OpenMetrics.
			if handleError(closer.Close()) {
				return
			}
		}

		handleError(lastErr)
	})

	if opts.Timeout <= 0 {
		return h
	}

	return fasthttp.TimeoutHandler(h, opts.Timeout, fmt.Sprintf(
		"Exceeded configured timeout of %v.\n",
		opts.Timeout,
	))
}
