package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vontikov/pgcluster/internal/app"
	"github.com/vontikov/pgcluster/internal/env"
	"github.com/vontikov/pgcluster/internal/gateway"
	"github.com/vontikov/pgcluster/internal/logging"
	"github.com/vontikov/pgcluster/internal/metric"
	"github.com/vontikov/pgcluster/internal/pg"
	"github.com/vontikov/pgcluster/internal/sentinel"
	"github.com/vontikov/pgcluster/internal/storage"
	"github.com/vontikov/pgcluster/internal/util"
)

const (
	defaultHttpPort        = "3501"
	defaultListenAddress   = "0.0.0.0"
	defaultLogLevel        = "info"
	defaultMetricsEnabled  = "true"
	defaultProfilerEnabled = "false"
)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	logLevel := env.GetOrDefault(env.LogLevel, defaultLogLevel)
	logging.SetLevel(logLevel)
	logger := logging.NewLogger("main")

	hostname, err := os.Hostname()
	util.PanicOnError(err)

	logger.Info("starting",
		"name", app.App,
		"namespace", app.Namespace,
		"version", app.Version,
		"hostname", hostname,
	)
	ctx, cancel := context.WithCancel(context.Background())

	cluster := pg.New(ctx,
		pg.WithDatabase(pg.DefaultDatabase),
		pg.WithHost(pg.DefaultHost),
		pg.WithPort(pg.DefaultPort),
		pg.WithUser(env.GetOrDefault(env.PgUser, pg.DefaultUser)),
		pg.WithPasswordFile(env.GetOrDefault(env.PgPasswordFile, pg.DefaultPasswordFile)),
	)

	metric.Init(ctx, hostname, cluster)

	major, minor, err := cluster.Version()
	util.PanicOnError(err)
	logger.Info("PostgreSQL version", "major", major, "minor", minor)

	storageBootstrap, err := env.Get(env.StorageBootstrap)
	util.PanicOnError(err)

	storageTtl, err := time.ParseDuration(env.GetOrDefault(env.StorageTtl, storage.DefaultTTL.String()))
	util.PanicOnError(err)

	storageClient, err := storage.New(ctx,
		storage.WithType(env.GetOrDefault(env.StorageType, storage.DefaultType)),
		storage.WithBootstrap(storageBootstrap),
		storage.WithTTL(storageTtl),
	)
	util.PanicOnError(err)

	s := sentinel.New(cluster, storageClient, hostname, pg.DefaultPort)
	err = s.Prepare(ctx)
	util.PanicOnError(err)

	gateway, err := gateway.New(ctx,
		gateway.WithLoggerName(app.App),
		gateway.WithHTTPPort(env.GetOrDefault(env.HttpPort, defaultHttpPort)),
		gateway.WithListenAddress(env.GetOrDefault(env.ListenAddress, defaultListenAddress)),
		gateway.WithMetricsEnabled(env.GetOrDefault(env.MetricsEnabled, defaultMetricsEnabled)),
		gateway.WithPprofEnabled(env.GetOrDefault(env.ProfilerEnabled, defaultProfilerEnabled)),
		gateway.WithHandlers(pg.Handlers(cluster)),
	)
	util.PanicOnError(err)

	go s.Start(ctx)
	metric.Info.Set(1.0)
	logger.Info("started")

	sig := <-signals
	logger.Debug("received signal", "type", sig)
	cancel()
	_ = gateway.Wait()
	logger.Info("done")
}
