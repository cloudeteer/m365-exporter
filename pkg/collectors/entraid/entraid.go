package entraid

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/cloudeteer/m365-exporter/pkg/util"
	abstractions "github.com/microsoft/kiota-abstractions-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cast"
)

const subsystem = "entraid"

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	userDesc *prometheus.Desc
}

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),

		userDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "user_count"),
			"User metrics in Entra ID",
			[]string{"type", "enabled"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
	}
}

func (c *Collector) StartBackgroundWorker(ctx context.Context, interval time.Duration) {
	go c.ScrapeWorker(ctx, c.logger, interval, c.ScrapeMetrics)
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.BaseCollector.Describe(ch)

	ch <- c.userDesc
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	errs := make([]error, 0)

	memberDisabledMetrics, err := c.scrapeUsers(ctx, false, "Member")
	if err != nil {
		errs = append(errs, fmt.Errorf("error scraping disabled member metrics: %w", err))
	}

	membereEnabledMetrics, err := c.scrapeUsers(ctx, true, "Member")
	if err != nil {
		errs = append(errs, fmt.Errorf("error scraping enabled member metrics: %w", err))
	}

	guestDisabledMetrics, err := c.scrapeUsers(ctx, false, "Guest")
	if err != nil {
		errs = append(errs, fmt.Errorf("error scraping disabled guest metrics: %w", err))
	}

	guestEnabledMetrics, err := c.scrapeUsers(ctx, true, "Guest")
	if err != nil {
		errs = append(errs, fmt.Errorf("error scraping enabled guest metrics: %w", err))
	}

	if err != nil {
		errs = append(errs, fmt.Errorf("error scraping user metrics: %w", err))
	}

	return slices.Concat(membereEnabledMetrics, memberDisabledMetrics, guestEnabledMetrics, guestDisabledMetrics), errors.Join(errs...)
}

func (c *Collector) scrapeUsers(ctx context.Context, userEnabled bool, userType string) ([]prometheus.Metric, error) {
	metrics := make([]prometheus.Metric, 0)

	filter := "accountEnabled eq " + cast.ToString(userEnabled) + " and userType eq '" + userType + "'"
	query := users.CountRequestBuilderGetQueryParameters{
		Filter: &filter,
	}

	headers := abstractions.NewRequestHeaders()
	headers.Add("ConsistencyLevel", "eventual")

	config := users.CountRequestBuilderGetRequestConfiguration{
		QueryParameters: &query,
		Headers:         headers,
	}

	all := c.GraphClient().Users().Count()

	result, err := all.Get(ctx, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to get managed user overview: %w", util.GetOdataError(err))
	}

	metrics = append(metrics, prometheus.MustNewConstMetric(
		c.userDesc,
		prometheus.GaugeValue,
		float64(*result),
		userType, cast.ToString(userEnabled),
	))

	return metrics, nil
}
