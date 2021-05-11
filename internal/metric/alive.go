package metric

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vontikov/pgcluster/internal/pg"
)

type livenessCollector struct {
	c pg.Cluster
	d *prometheus.Desc
}

func newLivenessCollector(hostname string, c pg.Cluster) *livenessCollector {
	return &livenessCollector{
		c: c,
		d: prometheus.NewDesc(
			QualifiedMetricName(IsAlive),
			"cluster liveness status",
			nil,
			map[string]string{hostnameLabel: hostname}),
	}
}

func (c *livenessCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.d
}

func (c *livenessCollector) Collect(ch chan<- prometheus.Metric) {
	var v float64
	r, err := c.c.Alive()
	if r {
		v = 1.0
	}
	if err != nil {
		v = -1.0
	}
	ch <- prometheus.MustNewConstMetric(c.d, prometheus.GaugeValue, v)
}
