package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

// NewGateMetrics creates new metrics for http gate.
func NewGateMetrics() *GateMetrics {
	stateMetric := newStateMetrics()
	stateMetric.register()

	return &GateMetrics{
		stateMetrics: *stateMetric,
	}
}

func newStateMetrics() *stateMetrics {
	return &stateMetrics{
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

// NewPrometheusService creates a new service for gathering prometheus metrics.
func NewPrometheusService(log *zap.Logger, cfg Config) *Service {
	if log == nil {
		return nil
	}

	return &Service{
		Server: &http.Server{
			Addr:    cfg.Address,
			Handler: promhttp.Handler(),
		},
		enabled:     cfg.Enabled,
		serviceType: "Prometheus",
		log:         log.With(zap.String("service", "Prometheus")),
	}
}
