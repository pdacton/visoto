package classstats

import "github.com/prometheus/client_golang/prometheus"

// Prometheus export of the latest snapshots (GET /metrics). Values are read at
// scrape time from memory; the endpoint label is the slug, never the URL.
var (
	descInstances = prometheus.NewDesc("visoto_class_instances",
		"Instances of a class in the latest daily snapshot (GraphDB statistics are approximate).",
		[]string{"endpoint", "class"}, nil)
	descTriples = prometheus.NewDesc("visoto_store_triples",
		"Triples in the store at the latest daily snapshot.",
		[]string{"endpoint"}, nil)
	descLastRun = prometheus.NewDesc("visoto_class_stats_last_run_timestamp_seconds",
		"When the latest class-stats run finished.",
		[]string{"endpoint"}, nil)
	descDuration = prometheus.NewDesc("visoto_class_stats_run_duration_seconds",
		"How long the latest class-stats run took.",
		[]string{"endpoint"}, nil)
	descClasses = prometheus.NewDesc("visoto_class_stats_classes",
		"Classes listed by the latest class-stats run.",
		[]string{"endpoint"}, nil)
	descFailures = prometheus.NewDesc("visoto_class_stats_failures",
		"Classes the latest class-stats run could not count (timeout or error).",
		[]string{"endpoint"}, nil)
)

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{descInstances, descTriples, descLastRun, descDuration, descClasses, descFailures} {
		ch <- d
	}
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for slug, s := range c.latest {
		for class, n := range s.Counts {
			ch <- prometheus.MustNewConstMetric(descInstances, prometheus.GaugeValue, float64(n), slug, class)
		}
		if s.Triples != nil {
			ch <- prometheus.MustNewConstMetric(descTriples, prometheus.GaugeValue, float64(*s.Triples), slug)
		}
		ch <- prometheus.MustNewConstMetric(descLastRun, prometheus.GaugeValue, float64(s.Finished.Unix()), slug)
		ch <- prometheus.MustNewConstMetric(descDuration, prometheus.GaugeValue, s.Finished.Sub(s.Started).Seconds(), slug)
		ch <- prometheus.MustNewConstMetric(descClasses, prometheus.GaugeValue, float64(s.Classes), slug)
		ch <- prometheus.MustNewConstMetric(descFailures, prometheus.GaugeValue, float64(s.Failures), slug)
	}
}
