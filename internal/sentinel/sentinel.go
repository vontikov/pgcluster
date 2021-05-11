package sentinel

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vontikov/pgcluster/internal/logging"
	"github.com/vontikov/pgcluster/internal/pg"
	"github.com/vontikov/pgcluster/internal/storage"
)

const (
	// DefaultLoggerName is the default name for the logger.
	DefaultLoggerName = "sentinel"

	// DefaultInterval is the default interval for the periodic checks.
	DefaultInterval = "1s"

	// DefaultPgAwaitTimeout is the default timeout for a replica to wait
	// the master to be ready.
	DefaultPgAwaitTimeout = 60 * time.Second

	DefaultPgPollDelay = 1 * time.Second
)

var dictKeyMasterInfo = []byte("master-info")

type hostinfo struct {
	Host string
	Port int
}

// ClusterState enumerates Cluster state.
type ClusterState int32

// Possible Cluster state.
const (
	Detached ClusterState = iota + 1
	Master
	Replica
)

// Sentinel watches the Cluster state.
type Sentinel struct {
	c        pg.Cluster
	storage  storage.Storage
	hostname string
	port     int
	interval time.Duration
	logger   logging.Logger
	errChan  chan error

	done int32

	mu                  sync.RWMutex // protects following fields
	state               ClusterState
	masterInfo          *hostinfo
	payload             []byte
	lastCheck           time.Time
	counterCheckSuccess int
	counterCheckErrors  int
}

// Option defines configuration option.
type Option func(*Sentinel)

// WithLoggerName sets logger name.
func WithLoggerName(v string) Option {
	return func(w *Sentinel) { w.logger = logging.NewLogger(v) }
}

// WithInterval sets watch interval.
func WithInterval(v string) Option {
	d, err := time.ParseDuration(v)
	return func(w *Sentinel) {
		if err != nil {
			return
		}
		w.interval = d
	}
}

// New creates new instance.
func New(c pg.Cluster, s storage.Storage, selfHost string, selfPgPort int, opts ...Option) *Sentinel {
	d, _ := time.ParseDuration(DefaultInterval)

	w := &Sentinel{
		c:        c,
		storage:  s,
		hostname: selfHost,
		port:     selfPgPort,

		logger:   logging.NewLogger(DefaultLoggerName),
		interval: d,

		errChan: make(chan error, 1),
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

// Prepare prepares the instance.
func (w *Sentinel) Prepare(ctx context.Context) error {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(&hostinfo{Host: w.hostname, Port: w.port})
	if err != nil {
		return err
	}
	w.payload = buf.Bytes()

	inRecovery, err := w.c.InRecovery()
	if err != nil {
		return err
	}
	w.logger.Info("cluster", "recovery mode", inRecovery)

	if !inRecovery {
		err = w.prepareMaster(ctx)
	} else {
		err = w.awaitMaster(ctx)
	}
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		close(w.errChan)
	}()

	return nil
}

func (w *Sentinel) prepareMaster(ctx context.Context) error {
	locked, err := w.storage.MutexTryLock(ctx)
	if err != nil {
		return err
	}
	w.logger.Debug("mutex", "locked", locked)

	// confirm master status
	if locked {
		w.mu.Lock()
		w.state = Master
		w.mu.Unlock()
		err := w.storage.DictionaryPut(ctx, dictKeyMasterInfo, w.payload)
		return err
	}

	// follow new master
	hi, err := w.getMaster(ctx)
	if err != nil {
		return err
	}
	if err := w.c.Stop(); err != nil {
		return err
	}
	if err := w.c.Backup(hi.Host, hi.Port); err != nil {
		return err
	}
	if err := w.c.Start(); err != nil {
		return err
	}
	w.mu.Lock()
	w.state = Replica
	w.mu.Unlock()
	return nil
}

func (w *Sentinel) awaitMaster(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, DefaultPgAwaitTimeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("master is not reachable within: %v", DefaultPgAwaitTimeout)
		default:
			masterInfo, err := w.c.MasterInfo()
			if err != nil {
				return err
			}
			if masterInfo != nil {
				w.mu.Lock()
				w.state = Replica
				w.masterInfo = &hostinfo{Host: masterInfo.Host, Port: masterInfo.Port}
				w.mu.Unlock()
				return nil
			}
			w.logger.Trace("master is not reachable yet")
			time.Sleep(DefaultPgPollDelay)
		}
	}
}

// State returns the observed Cluster state.
func (w *Sentinel) State() ClusterState {
	return ClusterState(atomic.LoadInt32((*int32)(&w.state)))
}

// Err returns the channel used to send errors while watching the Cluster state.
// The channel is closed when Wathchers stops.
func (w *Sentinel) Err() <-chan error {
	return w.errChan
}

// Reset resets the instance: counters, etc.
func (w *Sentinel) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.counterCheckSuccess = 0
	w.counterCheckErrors = 0
}

// Start runs the watching cycle.
func (w *Sentinel) Start(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.check(ctx)
		}
	}
}

func (w *Sentinel) check(ctx context.Context) {
	// make sure it is not executed while in progress
	if !atomic.CompareAndSwapInt32(&w.done, 0, 1) {
		w.logger.Trace("a task is in progress yet")
		return
	}
	defer atomic.StoreInt32(&w.done, 0)

	w.mu.Lock()
	defer w.mu.Unlock()

	var err error
	defer func() {
		if err != nil {
			w.counterCheckErrors++
			w.setErr(err)
		} else {
			w.counterCheckSuccess++
		}
		w.lastCheck = time.Now()
	}()

	switch w.state {
	case Master:
		err = w.checkMaster(ctx)
	case Replica:
		err = w.checkReplica(ctx)
	default:
		err = w.checkDetached(ctx)
	}
}

func (w *Sentinel) checkMaster(ctx context.Context) (err error) {
	r, err := w.c.Alive()
	if err != nil || !r {
		w.logger.Warn("master is down")
		if err = w.storage.MutexUnlock(ctx); err != nil {
			w.state = Detached
		}
		return
	}
	w.logger.Trace("master is up")
	return
}

func (w *Sentinel) checkReplica(ctx context.Context) error {
	w.logger.Trace("checking replica")
	r, err := w.c.Alive()
	if err != nil || !r {
		w.logger.Warn("replica is down")
	}
	w.logger.Trace("replica OK")

	locked, err := w.storage.MutexTryLock(ctx)
	if err != nil {
		w.logger.Warn("mutex eror", "message", err)
		return err
	}

	if locked {
		return w.promote(ctx)
	}
	return w.follow(ctx)
}

func (w *Sentinel) checkDetached(ctx context.Context) (err error) {
	r, err := w.c.Alive()
	if err != nil || !r {
		w.logger.Warn("instance is down")
		return
	}

	w.logger.Trace("instance is up")

	locked, err := w.storage.MutexTryLock(ctx)
	if err != nil {
		w.logger.Warn("mutex eror", "message", err)
		return err
	}

	if locked {
		return fmt.Errorf("inconsistent state")
	}

	if err = w.follow(ctx); err != nil {
		return
	}

	w.state = Replica
	return
}

func (w *Sentinel) promote(ctx context.Context) error {
	w.logger.Warn("promoting")

	// reset current master
	if err := w.storage.DictionaryRemove(ctx, dictKeyMasterInfo); err != nil {
		return err
	}

	if err := w.c.Promote(); err != nil {
		w.logger.Warn("failed to promote", "message", err)
		_ = w.storage.MutexUnlock(ctx)
		return err
	}

	w.logger.Debug("waiting recovery status to be changed")
	ctx, cancel := context.WithTimeout(ctx, DefaultPgAwaitTimeout)
	defer cancel()

loop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if r, err := w.c.InRecovery(); err == nil && !r {
				time.Sleep(DefaultPgPollDelay)
				break loop
			}
			w.logger.Debug("recovery status has not changed yet")
		}
	}

	w.state = Master
	if err := w.storage.DictionaryPut(ctx, dictKeyMasterInfo, w.payload); err != nil {
		return err
	}
	return nil
}

func (w *Sentinel) follow(ctx context.Context) error {
	actualMaster, err := w.getMaster(ctx)
	if err != nil {
		return err
	}

	if w.logger.IsTrace() {
		w.logger.Trace("actual master", "host", actualMaster.Host, "port", actualMaster.Port)
	}

	if w.masterInfo != nil && w.masterInfo.Host == actualMaster.Host && w.masterInfo.Port == actualMaster.Port {
		return nil
	}

	w.logger.Warn("master changed to", "host", actualMaster.Host, "port", actualMaster.Port)
	if err := w.c.Stop(); err != nil {
		w.state = Detached
		return err
	}

	if err := w.c.Backup(actualMaster.Host, actualMaster.Port); err != nil {
		w.state = Detached
		return err
	}
	if err := w.c.Start(); err != nil {
		w.state = Detached
		return err
	}
	w.masterInfo = &hostinfo{Host: actualMaster.Host, Port: actualMaster.Port}
	return nil
}

func (w *Sentinel) getMaster(ctx context.Context) (*hostinfo, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultPgAwaitTimeout)
	defer cancel()

	w.logger.Trace("receiving master info...")
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("master info is not available within: %v", DefaultPgAwaitTimeout)
		default:
			payload, err := w.storage.DictionaryGet(ctx, dictKeyMasterInfo)
			if err != nil {
				return nil, err
			}
			if payload != nil {
				var hi hostinfo
				if err := gob.NewDecoder(bytes.NewBuffer(payload)).Decode(&hi); err != nil {
					return nil, err
				}
				w.logger.Trace("received master info", "data", hi)
				return &hi, nil
			}
			w.logger.Trace("master info is not available yet")
			time.Sleep(DefaultPgPollDelay)
		}
	}
}

func (w *Sentinel) setErr(err error) {
	w.logger.Trace("registering error", "message", err)
	select {
	case w.errChan <- err:
	default:
	}
}
