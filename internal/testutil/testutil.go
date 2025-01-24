package testutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"
)

func MetricsToText(tb testing.TB, metrics []prometheus.Metric) (string, error) {
	tb.Helper()

	reg := prometheus.NewRegistry()
	reg.MustRegister(&collector{metrics: metrics})

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	require.NoError(tb, err)

	request.Header.Add("Accept", "test/plain")

	writer := httptest.NewRecorder()

	regHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	regHandler.ServeHTTP(writer, request)

	require.Equal(tb, http.StatusOK, writer.Code)

	allMetrics, err := io.ReadAll(writer.Body)
	if err != nil {
		return "", fmt.Errorf("error reading writer body: %w", err)
	}

	return string(allMetrics), nil
}

type collector struct {
	metrics []prometheus.Metric
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range c.metrics {
		ch <- metric.Desc()
	}
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	for _, metric := range c.metrics {
		ch <- metric
	}
}
