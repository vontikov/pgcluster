package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	assert := assert.New(t)

	const in = `
SUT:
  - image: github.com/vontikov/stoa:0.0.1
    container_name: stoa0
    hostname: stoa0
    command: stoa -bootstrap stoa0,stoa1,stoa2
  - image: github.com/vontikov/stoa:0.0.1
    container_name: stoa1
    hostname: stoa1
    command: stoa
  - image: github.com/vontikov/pgcluster:0.0.1
    container_name: pg_master
    hostname: pg_master
    environment:
      - PGCP_LOG_LEVEL=trace
      - PGCP_STOA_BOOTSTRAP=stoa0,stoa1,stoa2
      - PG_CLUSTER_NAME=master
      - PG_SYNC_NAMES=FIRST 1 (slave0)
    ports:
      - name: pg
        published: 5432
        target: 5432
      - name: mngmt
        published: 3501
        target: 3501
`

	cfg := &Config{}
	err := cfg.parse([]byte(in))
	assert.Nil(err)
	assert.Equal(defaultLoggingLevel, cfg.Logging.Level)
	assert.Equal(3, len(cfg.Container))

	c := cfg.Container[0]
	assert.Equal("github.com/vontikov/stoa:0.0.1", c.Image)
	assert.Equal("stoa0", c.Name)
	assert.Equal("stoa0", c.Hostname)
	assert.Equal("stoa -bootstrap stoa0,stoa1,stoa2", c.Command)

	c = cfg.Container[1]
	assert.Equal("github.com/vontikov/stoa:0.0.1", c.Image)
	assert.Equal("stoa1", c.Name)
	assert.Equal("stoa1", c.Hostname)
	assert.Equal("stoa", c.Command)

	c = cfg.Container[2]
	assert.Equal("github.com/vontikov/pgcluster:0.0.1", c.Image)
	assert.Equal("pg_master", c.Name)
	assert.Equal("pg_master", c.Hostname)
	assert.Equal("", c.Command)
	assert.Equal(
		[]string{
			"PGCP_LOG_LEVEL=trace",
			"PGCP_STOA_BOOTSTRAP=stoa0,stoa1,stoa2",
			"PG_CLUSTER_NAME=master",
			"PG_SYNC_NAMES=FIRST 1 (slave0)",
		}, c.Env)
	assert.Equal(
		[]Port{
			{"pg", 5432, 5432},
			{"mngmt", 3501, 3501},
		}, c.Ports)
}
