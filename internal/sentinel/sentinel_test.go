package sentinel

import (
	"bytes"
	"context"
	"encoding/gob"
	"testing"

	gm "github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
	"github.com/vontikov/pgcluster/internal/pg"

	mock_pg "github.com/vontikov/pgcluster/mocks/pg"
	mock_storage "github.com/vontikov/pgcluster/mocks/storage"
)

func TestPrepareMaster(t *testing.T) {
	const (
		selfHost    = "localhost"
		selfPort    = pg.DefaultPort
		inRecovery  = false
		mutexLocked = true
	)

	assert := assert.New(t)

	ctrl := gm.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var payload bytes.Buffer
	err := gob.NewEncoder(&payload).Encode(&hostinfo{selfHost, selfPort})
	assert.Nil(err)

	c := mock_pg.NewMockCluster(ctrl)
	s := mock_storage.NewMockStorage(ctrl)

	c.EXPECT().InRecovery().
		Return(inRecovery, nil).
		Times(1)
	s.EXPECT().MutexTryLock(ctx).
		Return(mutexLocked, nil).
		Times(1)
	s.EXPECT().DictionaryPut(ctx, dictKeyMasterInfo, payload.Bytes()).
		Return(nil).
		Times(1)

	w := New(c, s, selfHost, selfPort)
	assert.NotNil(w)

	err = w.Prepare(ctx)
	assert.Nil(err)
	assert.Equal(Master, w.State())
}

func TestPrepareReplica(t *testing.T) {
	const (
		selfHost   = "localhost"
		selfPort   = pg.DefaultPort
		inRecovery = true
	)

	assert := assert.New(t)

	ctrl := gm.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var payload bytes.Buffer
	err := gob.NewEncoder(&payload).Encode(&hostinfo{selfHost, selfPort})
	assert.Nil(err)

	c := mock_pg.NewMockCluster(ctrl)
	s := mock_storage.NewMockStorage(ctrl)

	c.EXPECT().InRecovery().
		Return(inRecovery, nil).
		Times(1)

	c.EXPECT().MasterInfo().
		Return(&pg.ConnectionInfo{}, nil).
		Times(1)

	w := New(c, s, selfHost, selfPort)
	assert.NotNil(w)

	err = w.Prepare(ctx)
	assert.Nil(err)
	assert.Equal(Replica, w.State())
}

func TestPrepareSecondMaster(t *testing.T) {
	const (
		selfHost    = "localhost"
		selfPort    = pg.DefaultPort
		inRecovery  = false
		mutexLocked = false
		otherHost   = "localhost"
		otherPort   = 5433
	)

	assert := assert.New(t)

	ctrl := gm.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var payload bytes.Buffer
	err := gob.NewEncoder(&payload).Encode(&hostinfo{otherHost, otherPort})
	assert.Nil(err)

	c := mock_pg.NewMockCluster(ctrl)
	s := mock_storage.NewMockStorage(ctrl)

	c.EXPECT().InRecovery().
		Return(inRecovery, nil).
		Times(1)
	s.EXPECT().MutexTryLock(ctx).
		Return(mutexLocked, nil).
		Times(1)
	s.EXPECT().DictionaryGet(gm.Any(), dictKeyMasterInfo).
		Return(payload.Bytes(), nil).
		Times(1)

	c.EXPECT().Stop().
		Return(nil).
		Times(1)
	c.EXPECT().Backup(otherHost, otherPort).
		Return(nil).
		Times(1)
	c.EXPECT().Start().
		Return(nil).
		Times(1)

	w := New(c, s, selfHost, selfPort)
	assert.NotNil(w)

	err = w.Prepare(ctx)
	assert.Nil(err)
	assert.Equal(Replica, w.State())
}
