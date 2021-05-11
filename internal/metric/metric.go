package metric

import (
	"context"
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/vontikov/pgcluster/internal/app"
	"github.com/vontikov/pgcluster/internal/pg"
)

const (
	InRecovery = "in_recovery"
	IsAlive    = "is_alive"

	versionLabel  = "version"
	hostnameLabel = "hostname"
)

var (
	// Info contains the app information.
	Info prometheus.Gauge

	once sync.Once
)

func Init(ctx context.Context, hostname string, cluster pg.Cluster) {
	once.Do(func() {
		Info = promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: app.Namespace,
			Subsystem: app.App,
			Name:      "info",
			Help:      "Application info",
			ConstLabels: prometheus.Labels{
				versionLabel:  app.Version,
				hostnameLabel: hostname,
			},
		})

		prometheus.MustRegister(newLivenessCollector(hostname, cluster))
		prometheus.MustRegister(newInRecoveryCollector(hostname, cluster))
	})
}

func QualifiedMetricName(metric string) string {
	return fmt.Sprintf("%s_%s_%s", app.Namespace, app.App, metric)
}
