package gateway

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGateway(t *testing.T) {
	assert := assert.New(t)

	const host = "localhost"
	const port = "5301"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	g, err := New(ctx, WithHTTPPort(port), WithMetricsEnabled("true"))
	assert.Nil(err)
	assert.NotNil(g)

	for {
		select {
		case <-ctx.Done():
			t.Errorf("timeout")
			t.Fail()
		default:
			url := fmt.Sprintf("http://%s:%s/metrics", host, port)
			if _, err := http.Get(url); err != nil {
				time.Sleep(200 * time.Millisecond)
				break
			}
			return
		}
	}
}
