package storage

import (
	"context"
	"time"

	"github.com/vontikov/pgcluster/internal/logging"
	stoa "github.com/vontikov/stoa/pkg/client"
)

const (
	DefaultStoaLoggerName = "stoa-storage"

	// DefaultStoaMutexName is the default name for the mutex locked by a master.
	DefaultStoaMutexName = "pg"

	// DefaultStoaDictionaryName is the default name for the dictionary used to
	// register clusters.
	DefaultStoaDictionaryName = "pg"
)

type stoaStorage struct {
	logger logging.Logger
	c      stoa.Client
	d      stoa.Dictionary
	m      stoa.Mutex
}

func newStoa(ctx context.Context, bootstrap string, ttl time.Duration) (Storage, error) {
	client, err := stoa.New(ctx,
		stoa.WithBootstrap(bootstrap),
		stoa.WithPingPeriod(ttl),
	)
	if err != nil {
		return nil, err
	}

	return &stoaStorage{
		logger: logging.NewLogger(DefaultStoaLoggerName),
		c:      client,
		m:      client.Mutex(DefaultStoaMutexName),
		d:      client.Dictionary(DefaultStoaDictionaryName),
	}, nil
}

func (s *stoaStorage) MutexTryLock(ctx context.Context) (locked bool, err error) {
	s.logger.Trace("trying to lock")
	locked, _, err = s.m.TryLock(ctx, nil)
	if err != nil {
		s.logger.Error("lock error", "message", err)
	}

	s.logger.Trace("lock", "result", locked)
	return
}

func (s *stoaStorage) MutexUnlock(ctx context.Context) (err error) {
	s.logger.Trace("unlocking")
	_, _, err = s.m.Unlock(ctx)
	if err != nil {
		s.logger.Error("unlock error", "message", err)
	}
	return
}

func (s *stoaStorage) DictionaryPut(ctx context.Context, k, v []byte) (err error) {
	s.logger.Trace("dictionary put")
	_, err = s.d.Put(ctx, k, v)
	if err != nil {
		s.logger.Error("dictionary put error", "message", err)
	}
	return
}

func (s *stoaStorage) DictionaryGet(ctx context.Context, k []byte) (r []byte, err error) {
	s.logger.Trace("dictionary get")
	r, err = s.d.Get(ctx, k)
	if err != nil {
		s.logger.Error("dictionary get error", "message", err)
	}
	return
}

func (s *stoaStorage) DictionaryRemove(ctx context.Context, k []byte) (err error) {
	s.logger.Trace("dictionary remove")
	err = s.d.Remove(ctx, k)
	if err != nil {
		s.logger.Error("dictionary remove error", "message", err)
	}
	return
}
