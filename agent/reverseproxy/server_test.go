package reverseproxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/andydunstall/piko/agent/config"
	"github.com/andydunstall/piko/pkg/log"
	"github.com/andydunstall/piko/pkg/middleware"
)

func mustGet(t *testing.T, url string) string {
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	if resp.Body != nil {
		defer func() {
			// nolint
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	return string(body)
}

func TestServer_Forward(t *testing.T) {
	// Issue https://github.com/andydunstall/piko/issues/216
	t.Run("multiendpoint", func(t *testing.T) {

		upstream := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				// nolint
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("bar"))
			},
		))
		defer upstream.Close()

		registry := prometheus.NewRegistry()
		metrics := middleware.NewMetrics("test")
		configs := []config.ListenerConfig{
			{
				EndpointID: "my-endpoint",
				Addr:       upstream.URL,
			},
			{
				EndpointID: "my-endpoint-2",
				Addr:       upstream.URL,
			},
		}

		testConfig := func(t *testing.T, cfg config.ListenerConfig) {
			// Need a real listener to test Server
			ln, err := net.Listen("tcp", ":0")
			if err != nil {
				panic(err)
			}
			defer ln.Close()
			lnPort := ln.Addr().(*net.TCPAddr).Port

			server := NewServer(cfg, metrics, log.NewNopLogger())
			go func() {
				server.Serve(ln)
			}()

			body := mustGet(t, fmt.Sprintf("http://localhost:%d/foo/bar?a=b", lnPort))
			assert.Equal(t, "bar", body)
		}
		metrics.Register(registry)

		t.Run("fist", func(t *testing.T) {
			testConfig(t, configs[0])
		})
		t.Run("second", func(t *testing.T) {
			testConfig(t, configs[1])
		})
	})
}
