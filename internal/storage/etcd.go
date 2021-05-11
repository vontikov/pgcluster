package storage

import (
	"context"
	"errors"
	"strings"
	"time"

	etcd "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"

	"github.com/vontikov/pgcluster/internal/logging"
)

const (
	DefaultEtcdLoggerName = "etcd-storage"

	DefaultEtcdMutexName = "pg"

	DefaultEtcdDictionaryName = "pg"

	DefaultEtcdDialTimeout      = 5000 * time.Millisecond
	DefaultEtcdOpTimeout        = 2000 * time.Millisecond
	DefaultEtcdKeepAliveTime    = 2000 * time.Millisecond
	DefaultEtcdKeepAliveTimeout = 1000 * time.Millisecond
)

type etcdStorage struct {
	logger logging.Logger
	m      *concurrency.Mutex
	kv     etcd.KV
}

func newEtcd(ctx context.Context, bootstrap string, ttl time.Duration) (Storage, error) {
	endpoints := strings.Split(bootstrap, ",")
	cli, err := etcd.New(
		etcd.Config{
			Endpoints:            endpoints,
			DialTimeout:          DefaultEtcdDialTimeout,
			DialKeepAliveTime:    DefaultEtcdKeepAliveTime,
			DialKeepAliveTimeout: DefaultEtcdKeepAliveTimeout,
		},
	)
	if err != nil {
		return nil, err
	}

	sess, err := concurrency.NewSession(cli, concurrency.WithTTL(int(ttl.Seconds())))
	if err != nil {
		return nil, err
	}

	kv := etcd.NewKV(cli)

	mux := concurrency.NewMutex(sess, DefaultEtcdMutexName)

	go func() {
		<-ctx.Done()
		_ = sess.Close()
		_ = cli.Close()
	}()

	return &etcdStorage{
		logger: logging.NewLogger(DefaultEtcdLoggerName),
		m:      mux,
		kv:     kv,
	}, nil
}

func (s *etcdStorage) MutexTryLock(ctx context.Context) (bool, error) {
	s.logger.Trace("trying to lock")
	ctx, cancel := context.WithTimeout(ctx, DefaultEtcdOpTimeout)
	defer cancel()

	if err := s.m.Lock(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return false, nil
		}
		s.logger.Error("lock error", "message", err)
		return false, err
	}

	s.logger.Trace("locked")
	return true, nil
}

func (s *etcdStorage) MutexUnlock(ctx context.Context) (err error) {
	s.logger.Trace("unlocking")
	ctx, cancel := context.WithTimeout(ctx, DefaultEtcdOpTimeout)
	defer cancel()

	err = s.m.Unlock(ctx)
	if err != nil {
		s.logger.Error("unlock error", "message", err)
	}
	s.logger.Trace("unlicked")
	return
}

func (s *etcdStorage) DictionaryPut(ctx context.Context, k, v []byte) (err error) {
	s.logger.Trace("dictionary put")
	ctx, cancel := context.WithTimeout(ctx, DefaultEtcdOpTimeout)
	defer cancel()

	_, err = s.kv.Put(ctx, string(k), string(v))
	if err != nil {
		s.logger.Error("dictionary put error", "message", err)
	}
	return
}

func (s *etcdStorage) DictionaryGet(ctx context.Context, k []byte) (r []byte, err error) {
	s.logger.Trace("dictionary get")
	ctx, cancel := context.WithTimeout(ctx, DefaultEtcdOpTimeout)
	defer cancel()

	gr, err := s.kv.Get(ctx, string(k))
	if err != nil {
		s.logger.Error("dictionary get error", "message", err)
		return
	}
	if len(gr.Kvs) > 0 {
		r = gr.Kvs[0].Value
	}
	return
}

func (s *etcdStorage) DictionaryRemove(ctx context.Context, k []byte) (err error) {
	s.logger.Trace("dictionary remove")
	ctx, cancel := context.WithTimeout(ctx, DefaultEtcdOpTimeout)
	defer cancel()

	_, err = s.kv.Delete(ctx, string(k))
	if err != nil {
		s.logger.Error("dictionary remove error", "message", err)
	}
	return
}
