package pg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cluster := New(ctx,
		WithHost(DefaultHost),
		WithPort(DefaultPort),
		WithDatabase(DefaultDatabase),
		WithUser(DefaultUser),
	)
	for i := 0; i < 10; i++ {
		major, minor, err := cluster.Version()
		assert.Nil(err)
		assert.Equal(imageMajorVersion, major)
		assert.Equal(imageMinorVersion, minor)
	}
}

func TestAlive(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cluster := New(ctx,
		WithHost(DefaultHost),
		WithPort(DefaultPort),
		WithDatabase(DefaultDatabase),
		WithUser(DefaultUser),
	)
	assert.True(cluster.Alive())
}

func TestInRecovery(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cluster := New(ctx,
		WithHost(DefaultHost),
		WithPort(DefaultPort),
		WithDatabase(DefaultDatabase),
		WithUser(DefaultUser),
	)
	r, err := cluster.InRecovery()
	assert.Nil(err)
	assert.False(r)
}
