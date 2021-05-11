package metric

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vontikov/pgcluster/internal/pg"
)

type inRecoveryCollector struct {
	c pg.Cluster
	d *prometheus.Desc
}

func newInRecoveryCollector(hostname string, c pg.Cluster) *inRecoveryCollector {
	return &inRecoveryCollector{
		c: c,
		d: prometheus.NewDesc(
			QualifiedMetricName(InRecovery),
			"cluster recovery status",
			nil,
			map[string]string{hostnameLabel: hostname}),
	}
}

func (c *inRecoveryCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.d
}

func (c *inRecoveryCollector) Collect(ch chan<- prometheus.Metric) {
	var v float64
	defer func() {
		ch <- prometheus.MustNewConstMetric(c.d, prometheus.GaugeValue, v)
	}()

	r, err := c.c.InRecovery()
	if err != nil {
		v = -1.0
		return
	}
	if r {
		v = 1.0
	}
}
