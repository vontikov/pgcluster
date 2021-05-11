package env

const (
	PgBackup          = "PG_BACKUP"
	PgData            = "PGDATA"
	PgBinDir          = "PG_BINDIR"
	PgHome            = "PG_HOME"
	PgPasswordFile    = "PG_PASSWORD_FILE"
	PgReplicationUser = "PG_REPLICATION_USER"
	PgUser            = "PG_USER"

	HttpPort        = "PGCP_HTTP_PORT"
	ListenAddress   = "PGCP_LISTEN_ADDR"
	LogLevel        = "PGCP_LOG_LEVEL"
	MetricsEnabled  = "PGCP_METRICS_ENABLED"
	ProfilerEnabled = "PGCP_PROFILER_ENABLED"

	StorageType      = "PGCP_STORAGE_TYPE"
	StorageBootstrap = "PGCP_STORAGE_BOOTSTRAP"
	StorageTtl       = "PGCP_STORAGE_TTL"
)
