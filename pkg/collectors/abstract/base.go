package abstract

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	Namespace = "m365"
)

type BaseCollector struct {
	msGraphClient *msgraphsdk.GraphServiceClient

	metrics   []prometheus.Metric
	collectMu *sync.RWMutex

	lastUpdateTimestamp   prometheus.Gauge
	scrapeDurationSeconds prometheus.Gauge
	scrapeSuccess         prometheus.Gauge

	subsystem string
}

func NewBaseCollector(msGraphClient *msgraphsdk.GraphServiceClient, collector string) BaseCollector {
	return BaseCollector{
		msGraphClient: msGraphClient,
		subsystem:     collector,
		lastUpdateTimestamp: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: "collector",
			Name:      "last_update_seconds_timestamp",
			Help:      "The timestamp of the last update of the metrics.",
			ConstLabels: map[string]string{
				"collector": collector,
			},
		}),
		scrapeDurationSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: "collector",
			Name:      "scrape_duration_seconds",
			Help:      "The duration of the last scrape.",
			ConstLabels: map[string]string{
				"collector": collector,
			},
		}),
		scrapeSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: "collector",
			Name:      "scrape_success",
			Help:      "Whether the scraper was successful.",
			ConstLabels: map[string]string{
				"collector": collector,
			},
		}),
		collectMu: &sync.RWMutex{},
	}
}

func (c *BaseCollector) Describe(ch chan<- *prometheus.Desc) {
	c.lastUpdateTimestamp.Describe(ch)
	c.scrapeDurationSeconds.Describe(ch)
	c.scrapeSuccess.Describe(ch)
}

func (c *BaseCollector) Collect(ch chan<- prometheus.Metric) {
	c.collectMu.RLock()

	ch <- c.lastUpdateTimestamp
	ch <- c.scrapeDurationSeconds
	ch <- c.scrapeSuccess

	for _, m := range c.metrics {
		ch <- m
	}

	c.collectMu.RUnlock()
}

func (c *BaseCollector) GraphClient() *msgraphsdk.GraphServiceClient {
	return c.msGraphClient
}

func (c *BaseCollector) GetSubsystem() string {
	return c.subsystem
}

func (c *BaseCollector) ScrapeWorker(
	ctx context.Context, logger *slog.Logger, interval time.Duration, function func(ctx context.Context,
	) ([]prometheus.Metric, error),
) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic in scrapeWorker",
				slog.Any("err", r),
				slog.String("stack", string(debug.Stack())),
			)

			// if there is a go panic, restart the ScrapeWorker
			c.ScrapeWorker(ctx, logger, interval, function)
		}
	}()

	for {
		logger.DebugContext(ctx, "starting scrapeWorker")

		now := time.Now()
		prometheusMetrics, err := function(ctx)

		duration := time.Since(now)
		c.scrapeDurationSeconds.Set(duration.Seconds())

		if err != nil {
			c.scrapeSuccess.Set(0)
			logger.ErrorContext(ctx, fmt.Sprintf("collector failed after %s, resulting in %d metrics", duration, len(prometheusMetrics)),
				slog.Any("err", err),
			)
		} else {
			c.scrapeSuccess.Set(1)
			c.setMetrics(prometheusMetrics)

			logger.DebugContext(ctx, fmt.Sprintf("collector succeeded after %s, resulting in %d metrics", duration, len(prometheusMetrics)))
		}

		select {
		case <-time.After(interval):
			// scrape again
		case <-ctx.Done():
			return
		}
	}
}

func (c *BaseCollector) setMetrics(metrics []prometheus.Metric) {
	c.collectMu.Lock()
	c.metrics = metrics

	c.lastUpdateTimestamp.SetToCurrentTime()
	c.collectMu.Unlock()
}
