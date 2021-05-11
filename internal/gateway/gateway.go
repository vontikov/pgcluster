package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vontikov/pgcluster/internal/logging"
	"github.com/vontikov/pgcluster/internal/util"
)

const (
	readTimeout    = 10 * time.Second
	writeTimeout   = 10 * time.Second
	maxHeaderBytes = 1 << 20
)

type options struct {
	httpPort        int
	ip              string
	loggerName      string
	metricsEnabled  bool
	profilerEnabled bool
	handlers        map[string]func(http.ResponseWriter, *http.Request)
}

// Gateway processes external requests.
type Gateway = errgroup.Group

// Option defines Gateway configuration option.
type Option func(*options)

// WithListenAddress sets the Gateway listen address.
func WithListenAddress(v string) Option {
	return func(o *options) { o.ip = v }
}

// WithLoggerName sets the Gateway logger name.
func WithLoggerName(v string) Option {
	return func(o *options) { o.loggerName = v }
}

// WithHTTPPort sets the Gateway http port.
func WithHTTPPort(v string) Option {
	r, err := strconv.Atoi(v)
	util.PanicOnError(err)
	return func(o *options) { o.httpPort = r }
}

// WithMetricsEnabled enables or disables the Gateway metrics.
func WithMetricsEnabled(v string) Option {
	return func(o *options) { o.metricsEnabled = strings.ToLower(v) == "true" }
}

// WithPprofEnabled enables or disables the Gateway pprof.
func WithPprofEnabled(v string) Option {
	return func(o *options) { o.profilerEnabled = strings.ToLower(v) == "true" }
}

// WithHandlers provides additional handlers to the Gateway.
func WithHandlers(m map[string]func(http.ResponseWriter, *http.Request)) Option {
	return func(o *options) {
		if o.handlers == nil {
			o.handlers = make(map[string]func(http.ResponseWriter, *http.Request))
		}
		for k, v := range m {
			o.handlers[k] = v
		}
	}
}

// New creates new Gateway instance
func New(ctx context.Context, opts ...Option) (*Gateway, error) {
	cfg := &options{}
	for _, o := range opts {
		o(cfg)
	}

	logger := logging.NewLogger(cfg.loggerName)

	httpAddr := fmt.Sprintf("%s:%d", cfg.ip, cfg.httpPort)

	mux := http.NewServeMux()
	registerHandlers(mux, cfg)

	httpServer := &http.Server{
		Addr:           httpAddr,
		Handler:        mux,
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		MaxHeaderBytes: maxHeaderBytes,
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		logger.Info("serving http", "address", httpAddr)
		defer logger.Info("http stopped")
		return httpServer.ListenAndServe()
	})
	g.Go(func() error {
		<-ctx.Done()
		_ = httpServer.Close()
		return nil
	})

	return g, nil
}

func registerHandlers(mux *http.ServeMux, cfg *options) {
	if cfg.metricsEnabled {
		mux.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)
	}

	if cfg.profilerEnabled {
		mux.HandleFunc("/debug/pprof", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

		mux.HandleFunc("/profile", pprof.Profile)
		mux.Handle("/profile/block", pprof.Handler("block"))
		mux.Handle("/profile/goroutine", pprof.Handler("goroutine"))
		mux.Handle("/profile/heap", pprof.Handler("heap"))
		mux.Handle("/profile/mutex", pprof.Handler("mutex"))
		mux.Handle("/profile/threadcreate", pprof.Handler("threadcreate"))
	}

	for k, v := range cfg.handlers {
		mux.HandleFunc(k, v)
	}
}
