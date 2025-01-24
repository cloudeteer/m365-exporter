package abstract

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Collector interface {
	prometheus.Collector

	StartBackgroundWorker(ctx context.Context, interval time.Duration)
	ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error)
}
