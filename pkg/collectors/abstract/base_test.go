package abstract_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewBaseCollector(t *testing.T) {
	collector := abstract.NewBaseCollector(nil, "test")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fn := func(ctx context.Context) ([]prometheus.Metric, error) {
		metric, err := prometheus.NewConstMetric(
			prometheus.NewDesc("test_metric", "test", nil, nil),
			prometheus.CounterValue, 1,
		)
		if err != nil {
			return nil, err
		}

		return []prometheus.Metric{metric}, nil
	}

	go collector.ScrapeWorker(ctx, logger, time.Second, fn)

	time.Sleep(500 * time.Microsecond)

	reg := prometheus.NewRegistry()
	reg.MustRegister(&collector)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	metrics := strings.Builder{}

	for _, mf := range mfs {
		metrics.WriteString(mf.String())
	}

	stringMetrics := metrics.String()

	require.NotEmpty(t, stringMetrics)
	assert.Contains(t, stringMetrics, "test_metric")
	assert.Contains(t, stringMetrics, "m365_collector_last_update_seconds_timestamp")
	assert.Contains(t, stringMetrics, "m365_collector_scrape_duration_seconds")
	assert.Contains(t, stringMetrics, "m365_collector_scrape_success")
}
