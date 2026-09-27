package monitor

import "github.com/prometheus/client_golang/prometheus"

// Prometheus export of the latest probe per endpoint (GET /metrics). The
// endpoint label is the slug; endpoint URLs stay server-side.
var (
	descUp = prometheus.NewDesc("visoto_endpoint_up",
		"1 if the latest probe of the endpoint succeeded, else 0.",
		[]string{"endpoint"}, nil)
	descResponse = prometheus.NewDesc("visoto_endpoint_response_seconds",
		"Response time of the latest successful probe.",
		[]string{"endpoint"}, nil)
	descEnabled = prometheus.NewDesc("visoto_monitoring_enabled",
		"1 while endpoint monitoring is switched on.",
		nil, nil)
)

// Describe implements prometheus.Collector.
func (m *Monitor) Describe(ch chan<- *prometheus.Desc) {
	ch <- descUp
	ch <- descResponse
	ch <- descEnabled
}

// Collect implements prometheus.Collector.
func (m *Monitor) Collect(ch chan<- prometheus.Metric) {
	enabled := 0.0
	if m.IsEnabled() {
		enabled = 1
	}
	ch <- prometheus.MustNewConstMetric(descEnabled, prometheus.GaugeValue, enabled)

	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ep := range m.cfg.SparqlEndpoints {
		metric, ok := m.latest[ep.URL]
		if !ep.Monitor || !ok || ep.Slug == "" {
			continue
		}
		up := 0.0
		if metric.Status == "ok" {
			up = 1
			ch <- prometheus.MustNewConstMetric(descResponse, prometheus.GaugeValue, float64(metric.DurationMs)/1000, ep.Slug)
		}
		ch <- prometheus.MustNewConstMetric(descUp, prometheus.GaugeValue, up, ep.Slug)
	}
}
