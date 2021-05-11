package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/prometheus/common/expfmt"
	"github.com/vontikov/pgcluster/internal/docker"
	"github.com/vontikov/pgcluster/internal/logging"
	"github.com/vontikov/pgcluster/internal/metric"
	"github.com/vontikov/pgcluster/internal/pg"
	"github.com/vontikov/pgcluster/internal/util"
)

const (
	loggerName      = "test"
	networkNameSize = 16
	randStringSrc   = "abcdef1234567890"
	hostIP          = "0.0.0.0"
)

const (
	DefaultHost              = "localhost"
	DefaultPort              = "5432"
	DefaultDatabase          = "postgres"
	DefaultUser              = "postgres"
	DefaultPassword          = "12345"
	DefaultConnectionTimeout = 10 * time.Second
	DefaultQueryTimeout      = 10 * time.Second
)

type (
	metricValue  = float64
	serviceName  = string
	servicePort  = int
	containerID  = string
	serviceState int
)

const (
	Master serviceState = iota + 1
	Replica
	Stopped
	Unavailable
)

func serviceStateFromString(s string) serviceState {
	switch s {
	case "master":
		return Master
	case "replica":
		return Replica
	case "stopped":
		return Stopped
	default:
		return Unavailable
	}
}

func (s serviceState) String() string {
	switch s {
	case Master:
		return "master"
	case Replica:
		return "replica"
	case Stopped:
		return "stopped"
	case Unavailable:
		return "unavailable"
	default:
		return "unknown"
	}
}

// TestHandle contains the test context.
type TestHandle struct {
	logger logging.Logger
	dc     *docker.Client
	cfg    *Config

	network   string
	networkID string

	expectedInitialRecoveryStatus map[serviceName]bool

	mgmtPorts     map[serviceName]servicePort
	pgPorts       map[serviceName]servicePort
	containerIDs  map[serviceName]containerID
	serviceStates map[serviceName]serviceState

	host              string
	db                string
	user              string
	password          string
	connectionTimeout time.Duration
	queryTimeout      time.Duration
}

// NewTestHandle creates TestContext instance.
func NewTestHandle(cfg *Config) (*TestHandle, error) {
	dockerCli, err := docker.New()
	if err != nil {
		return nil, err
	}

	return &TestHandle{
		logger: logging.NewLogger(loggerName),
		cfg:    cfg,
		dc:     dockerCli,

		expectedInitialRecoveryStatus: map[serviceName]bool{"master": false, "replica": true},

		containerIDs:  make(map[serviceName]containerID),
		mgmtPorts:     make(map[serviceName]servicePort),
		pgPorts:       make(map[serviceName]servicePort),
		serviceStates: make(map[serviceName]serviceState),

		host:              DefaultHost,
		db:                DefaultDatabase,
		user:              DefaultUser,
		password:          DefaultPassword,
		connectionTimeout: DefaultConnectionTimeout,
		queryTimeout:      DefaultQueryTimeout,
	}, nil
}

func (h *TestHandle) Start() (err error) {
	h.network, err = util.RandString(networkNameSize, randStringSrc)
	if err != nil {
		return
	}
	if h.networkID, err = h.dc.NetworkCreate(h.network); err != nil {
		return
	}

	for _, ct := range h.cfg.Container {
		var ports [][]int
		for _, p := range ct.Ports {
			ports = append(ports, []int{p.Published, p.Target})

			if p.Name == ManagementPortName {
				h.mgmtPorts[ct.Hostname] = p.Published
			}
			if p.Name == PgPortName {
				h.pgPorts[ct.Hostname] = p.Published
			}
		}

		options := docker.ContainerOptions{
			Image:    ct.Image,
			Env:      ct.Env,
			HostIP:   hostIP,
			Ports:    ports,
			Volumes:  ct.Volumes,
			Hostname: ct.Hostname,
			Command:  ct.Command,
			Name:     ct.Name,
			Network:  h.network,
		}
		var id string
		if id, err = h.dc.ContainerCreate(options); err != nil {
			return
		}
		h.containerIDs[ct.Hostname] = id
		if err = h.dc.ContainerStart(id); err != nil {
			return
		}
	}
	return
}

func (h *TestHandle) Stop() (err error) {
	for _, id := range h.containerIDs {
		if err = h.dc.ContainerRemove(id); err != nil {
			return
		}
	}

	err = h.dc.NetworkRemove(h.networkID)
	return
}

func (h *TestHandle) getMgmtPort(service serviceName) (port servicePort, err error) {
	port, ok := h.mgmtPorts[service]
	if !ok {
		err = fmt.Errorf("management port not found for service %s", service)
	}
	return
}

func (h *TestHandle) getPgPort(service serviceName) (port servicePort, err error) {
	port, ok := h.pgPorts[service]
	if !ok {
		err = fmt.Errorf("postgresql port not found for service %s", service)
	}
	return
}

func (h *TestHandle) getMetric(service, metricName string) (r float64, err error) {
	port, err := h.getMgmtPort(service)
	if err != nil {
		return
	}

	url := fmt.Sprintf("http://localhost:%d/metrics", port)
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var parser expfmt.TextParser
	metrics, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		return
	}

	qn := metric.QualifiedMetricName(metricName)
	m, ok := metrics[qn]
	if !ok {
		err = fmt.Errorf("metric not found: %s", qn)
		return
	}
	for _, v := range m.Metric {
		return *v.Gauge.Value, nil
	}
	err = fmt.Errorf("metric value not found: %s", qn)
	return
}

func (h *TestHandle) getMasterInfo(service string) (r *pg.ConnectionInfo, err error) {
	port, err := h.getMgmtPort(service)
	if err != nil {
		return
	}

	url := fmt.Sprintf("http://localhost:%d/pg/masterinfo", port)
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	var hi pg.ConnectionInfo
	err = json.Unmarshal(b, &hi)
	r = &hi
	return
}

func (h *TestHandle) connGet(service serviceName) (*pgx.Conn, error) {
	port, err := h.getPgPort(service)
	if err != nil {
		return nil, err
	}

	connStr := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s", h.user, h.password, h.host, port, h.db)
	h.logger.Debug("connecting to database", "connection", connStr)
	ctx, cancel := context.WithTimeout(context.Background(), h.connectionTimeout)
	defer cancel()
	return pgx.Connect(ctx, connStr)
}
