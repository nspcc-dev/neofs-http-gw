package main

import (
	"github.com/fasthttp/router"
	"github.com/nspcc-dev/neofs-http-gw/response"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

const (
	namespace      = "neofs_http_gw"
	stateSubsystem = "state"
)

type GateMetrics struct {
	stateMetrics
}

type stateMetrics struct {
	healthCheck prometheus.Gauge
}

func newGateMetrics() *GateMetrics {
	stateMetric := newStateMetrics()
	stateMetric.register()

	return &GateMetrics{
		stateMetrics: stateMetric,
	}
}

func newStateMetrics() stateMetrics {
	return stateMetrics{
		healthCheck: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: stateSubsystem,
			Name:      "health",
			Help:      "Current HTTP gateway state",
		}),
	}
}

func (m stateMetrics) register() {
	prometheus.MustRegister(m.healthCheck)
}

func (m stateMetrics) SetHealth(s int32) {
	m.healthCheck.Set(float64(s))
}

func attachMetrics(r *router.Router, l *zap.Logger) {
	r.GET("/metrics/", metricsHandler(prometheus.DefaultGatherer, l))
}

func metricsHandler(reg prometheus.Gatherer, logger *zap.Logger) fasthttp.RequestHandler {
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

	h := fasthttp.RequestHandler(func(c *fasthttp.RequestCtx) {
		if inFlightSem != nil {
			select {
			case inFlightSem <- struct{}{}: // All good, carry on.
				defer func() { <-inFlightSem }()
			default:
				response.Error(c, "Limit of concurrent requests reached, try again later.", fasthttp.StatusServiceUnavailable)
				return
			}
		}
		mfs, err := reg.Gather()
		if err != nil {
			if logger != nil {
				panic("error gathering metrics:" + err.Error())
			}

			errCnt.WithLabelValues("gathering").Inc()
			response.Error(c, err.Error(), fasthttp.StatusServiceUnavailable)
			return
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
			if logger != nil {
				logger.Error("encoding and sending metric family", zap.Error(err))
			}
			errCnt.WithLabelValues("encoding").Inc()
			response.Error(c, err.Error(), fasthttp.StatusServiceUnavailable)
			return true
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

	return h
}
