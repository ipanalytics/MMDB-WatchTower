package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry      *prometheus.Registry
	success       *prometheus.CounterVec
	failure       *prometheus.CounterVec
	age           *prometheus.GaugeVec
	size          *prometheus.GaugeVec
	buildEpoch    *prometheus.GaugeVec
	rollbackTotal *prometheus.CounterVec
}

func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		success: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mmdbwatch_update_success_total",
			Help: "Successful MMDB updates.",
		}, []string{"name"}),
		failure: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mmdbwatch_update_failure_total",
			Help: "Failed MMDB updates.",
		}, []string{"name", "reason"}),
		age: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "mmdbwatch_database_age_seconds",
			Help: "Current database age in seconds.",
		}, []string{"name"}),
		size: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "mmdbwatch_database_size_bytes",
			Help: "Current database size in bytes.",
		}, []string{"name"}),
		buildEpoch: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "mmdbwatch_current_build_epoch",
			Help: "Current MMDB build epoch.",
		}, []string{"name"}),
		rollbackTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mmdbwatch_rollback_total",
			Help: "MMDB rollbacks.",
		}, []string{"name"}),
	}
	m.registry.MustRegister(m.success, m.failure, m.age, m.size, m.buildEpoch, m.rollbackTotal)
	return m
}

func (m *Metrics) Success(name string) { m.success.WithLabelValues(name).Inc() }
func (m *Metrics) Failure(name, reason string) {
	m.failure.WithLabelValues(name, reason).Inc()
}
func (m *Metrics) Rollback(name string) { m.rollbackTotal.WithLabelValues(name).Inc() }
func (m *Metrics) SetDatabase(name string, ageSeconds, sizeBytes, buildEpoch float64) {
	m.age.WithLabelValues(name).Set(ageSeconds)
	m.size.WithLabelValues(name).Set(sizeBytes)
	m.buildEpoch.WithLabelValues(name).Set(buildEpoch)
}

func Serve(addr string, m *Metrics) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))
	return http.ListenAndServe(addr, mux)
}
