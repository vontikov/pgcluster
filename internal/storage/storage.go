package storage

import (
	"context"
	"time"
)

type Storage interface {
	MutexTryLock(context.Context) (bool, error)

	MutexUnlock(context.Context) error

	DictionaryPut(ctx context.Context, k, v []byte) error

	DictionaryGet(ctx context.Context, k []byte) ([]byte, error)

	DictionaryRemove(ctx context.Context, k []byte) error
}

const (
	Stoa = "stoa"
	Etcd = "etcd"
)

const (
	DefaultType = Stoa
	DefaultTTL  = 5000 * time.Millisecond
)

type options struct {
	t         string
	ttl       time.Duration
	bootstrap string
}

type Option func(*options)

func defaultOptions() *options {
	return &options{
		t:   DefaultType,
		ttl: DefaultTTL,
	}
}

func WithBootstrap(v string) Option  { return func(o *options) { o.bootstrap = v } }
func WithTTL(v time.Duration) Option { return func(o *options) { o.ttl = v } }
func WithType(v string) Option       { return func(o *options) { o.t = v } }

func New(ctx context.Context, opts ...Option) (Storage, error) {
	cfg := defaultOptions()
	for _, o := range opts {
		o(cfg)
	}

	switch cfg.t {
	case Stoa:
		return newStoa(ctx, cfg.bootstrap, cfg.ttl)
	case Etcd:
		return newEtcd(ctx, cfg.bootstrap, cfg.ttl)
	}
	panic("unknown storage type")
}
