package pg

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/vontikov/pgcluster/internal/env"
	"github.com/vontikov/pgcluster/internal/logging"
	"github.com/vontikov/pgcluster/internal/util"
)

const (
	DefaultHost              = "localhost"
	DefaultPort              = 5432
	DefaultDatabase          = "postgres"
	DefaultUser              = "postgres"
	DefaultLoggerName        = "pg"
	DefaultConnectionTimeout = 30 * time.Second
	DefaultPromotionTimeout  = 30 * time.Second
)

var (
	DefaultPasswordFile           string = "N/A"
	ReplicationPromoteTriggerFile string = "N/A"
)

var (
	ErrNotInRecovery    = errors.New("not in recovery")
	ErrPromotionTimeout = errors.New("promotion timeout")
)

func init() {
	pghome, err := env.Get(env.PgHome)
	if err != nil {
		return
	}
	DefaultPasswordFile = pghome + "/pwfile"

	pgdata, err := env.Get(env.PgData)
	if err != nil {
		return
	}
	ReplicationPromoteTriggerFile = pgdata + "/promote.signal"
}

// ConnectionInfo contains connection info.
type ConnectionInfo struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// Cluster provides access to database.
type Cluster interface {
	// Version returns Cluster version.
	Version() (major int, minor int, err error)

	// Alive returns Cluster liveness status.
	Alive() (bool, error)

	// InRecovery returns Cluster recovery status.
	InRecovery() (bool, error)

	// MasterInfo returns the master info Cluster is a replica of.
	MasterInfo() (*ConnectionInfo, error)

	// Stop stops Cluster.
	Stop() error

	// Start starts Cluster.
	Start() error

	// Promote promotes standby to master.
	Promote() error

	// Backup backs up Cluster from the host:port.
	Backup(host string, port int) error
}

// Option defines configuration option.
type Option func(*cluster)

// WithLoggerName sets logger name.
func WithLoggerName(v string) Option { return func(c *cluster) { c.logger = logging.NewLogger(v) } }

// WithHost sets PostgreSQL host.
func WithHost(v string) Option { return func(c *cluster) { c.host = v } }

// WithPort sets PostgreSQL host.
func WithPort(v int) Option { return func(c *cluster) { c.port = v } }

// WithDatabase sets PostgreSQL database to connect.
func WithDatabase(v string) Option { return func(c *cluster) { c.db = v } }

// WithUser sets the user to connect with.
func WithUser(v string) Option { return func(c *cluster) { c.user = v } }

// WithPassword sets the user password.
func WithPassword(v string) Option { return func(c *cluster) { c.password = v } }

// WithPasswordFile provides password file.
func WithPasswordFile(v string) Option {
	/* #nosec */
	f, err := os.Open(v)
	util.PanicOnError(err)
	sc := bufio.NewScanner(f)
	if r := sc.Scan(); !r {
		util.PanicOnError(sc.Err())
	}
	return func(c *cluster) { c.password = sc.Text() }
}

// cluster gives access to the local PostgreSQL cluster.
// Provides convenience functions.
type cluster struct {
	ctx context.Context

	host     string
	port     int
	db       string
	user     string
	password string

	logger            logging.Logger
	connectionTimeout time.Duration
	promotionTimeout  time.Duration

	mu   sync.Mutex // protects following fields
	pool *pgxpool.Pool
}

// New returns new Cluster.
func New(ctx context.Context, opts ...Option) Cluster {
	c := &cluster{
		ctx:               ctx,
		logger:            logging.NewLogger(DefaultLoggerName),
		connectionTimeout: DefaultConnectionTimeout,
		promotionTimeout:  DefaultPromotionTimeout,
	}

	for _, o := range opts {
		o(c)
	}

	go func() {
		<-ctx.Done()
		c.poolDrop()
	}()

	return c
}

// Version implements Cluster.Version().
func (c *cluster) Version() (major int, minor int, err error) {
	const sql = "SELECT version();"
	const pattern = `PostgreSQL (\d+)\.(\d+).*`

	pool, err := c.poolGetOrConnect()
	if err != nil {
		c.logger.Error("connection error", "message", err)
		c.poolDrop()
		return
	}

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		return
	}
	defer conn.Release()

	var s string
	err = conn.QueryRow(c.ctx, sql).Scan(&s)
	if err != nil {
		return
	}

	re := regexp.MustCompile(pattern)
	m := re.FindAllSubmatch([]byte(s), -1)

	major, err = strconv.Atoi(string(m[0][1]))
	if err != nil {
		return
	}
	minor, err = strconv.Atoi(string(m[0][2]))
	return
}

// Alive implements Cluster.Alive().
func (c *cluster) Alive() (r bool, err error) {
	const sql = "SELECT true"

	pool, err := c.poolGetOrConnect()
	if err != nil {
		c.logger.Error("connection error", "message", err)
		c.poolDrop()
		return
	}

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		return
	}
	defer conn.Release()

	err = conn.QueryRow(c.ctx, sql).Scan(&r)
	return
}

// InRecovery implements Cluster.InRecovery().
func (c *cluster) InRecovery() (r bool, err error) {
	const sql = "SELECT pg_is_in_recovery()"

	pool, err := c.poolGetOrConnect()
	if err != nil {
		c.logger.Error("connection error", "message", err)
		c.poolDrop()
		return
	}

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		return
	}
	defer conn.Release()

	err = conn.QueryRow(c.ctx, sql).Scan(&r)
	return
}

// MasterInfo implements Cluster.MasterInfo().
func (c *cluster) MasterInfo() (hi *ConnectionInfo, err error) {
	const sql = "SELECT sender_host, sender_port from pg_stat_wal_receiver"

	pool, err := c.poolGetOrConnect()
	if err != nil {
		c.logger.Error("connection error", "message", err)
		c.poolDrop()
		return
	}

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		return
	}
	defer conn.Release()

	rows, err := conn.Query(c.ctx, sql)
	if err != nil {
		return
	}
	defer rows.Close()

	if !rows.Next() {
		return
	}

	vals, err := rows.Values()
	if err != nil {
		return
	}
	hi = &ConnectionInfo{
		Host: vals[0].(string),
		Port: int(vals[1].(int32)),
	}
	return
}

// Stop implements Cluster.Stop().
func (c *cluster) Stop() (err error) {
	c.logger.Warn("stopping cluster")
	dir, err := env.Get(env.PgBinDir)
	if err != nil {
		return
	}
	cmd := exec.Command(fmt.Sprintf("%s/pg_ctl", dir), "stop")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		c.logger.Error("command error", "message", err)
	}
	return
}

// Start implements Cluster.Start().
func (c *cluster) Start() (err error) {
	c.logger.Warn("starting cluster")
	dir, err := env.Get(env.PgBinDir)
	if err != nil {
		return
	}
	cmd := exec.Command(fmt.Sprintf("%s/pg_ctl", dir), "start")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		c.logger.Error("command error", "message", err)
	}
	return
}

// Promote implements Cluster.Promote().
func (c *cluster) Promote() (err error) {
	r, err := c.InRecovery()
	if err != nil {
		return
	}
	if !r {
		return ErrNotInRecovery
	}

	c.logger.Debug("creating trigger", "name", ReplicationPromoteTriggerFile)
	f, err := os.Create(ReplicationPromoteTriggerFile)
	if err != nil {
		return
	}
	err = f.Close()
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(c.ctx, DefaultPromotionTimeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ErrPromotionTimeout
		default:
			r, err = c.InRecovery()
			if err != nil {
				return
			}
			if !r {
				return
			}
			time.Sleep(1 * time.Second)
		}
	}
}

//  Backup implements Cluster.Backup().
func (c *cluster) Backup(host string, port int) (err error) {
	c.logger.Warn("backup from", "host", host, "port", port)

	user, err := env.Get(env.PgReplicationUser)
	if err != nil {
		return
	}
	c.logger.Debug("replication user", "name", user)

	dataDir, err := env.Get(env.PgData)
	if err != nil {
		return
	}
	c.logger.Debug("data directory", "path", dataDir)

	backupRoot, err := env.Get(env.PgBackup)
	if err != nil {
		return
	}
	backupDir := fmt.Sprintf("%s/%s", backupRoot, time.Now().Format("20060102150405"))
	c.logger.Debug("backup directory", "path", backupDir)

	err = os.Rename(dataDir, backupDir)
	if err != nil {
		c.logger.Error("backup error", "message", err)
		return
	}

	args := []string{
		"-h", host,
		"-p", strconv.Itoa(port),
		"-U", user,
		"-D", dataDir,
		"-P",
		"-R",
		"-v",
	}

	cmd := exec.Command("pg_basebackup", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	return
}

func (c *cluster) poolGetOrConnect() (pool *pgxpool.Pool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pool != nil {
		if c.logger.IsTrace() {
			c.logger.Trace("reusing the connection")
		}
		return c.pool, nil
	}

	c.logger.Debug("opening connection...")
	ctx, cancel := context.WithTimeout(context.Background(), c.connectionTimeout)
	defer cancel()

	connStr := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s", c.user, c.password, c.host, c.port, c.db)
	if pool, err = pgxpool.Connect(ctx, connStr); err == nil {
		c.pool = pool
		c.logger.Debug("connection established")
		return
	}

	t := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			if pool, err = pgxpool.Connect(ctx, connStr); err == nil {
				c.pool = pool
				t.Stop()
				c.logger.Debug("connection established")
				return
			}
		}
	}
}

func (c *cluster) poolDrop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pool != nil {
		c.pool.Close()
	}
	c.pool = nil
}
