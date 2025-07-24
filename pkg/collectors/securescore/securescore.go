package securescore

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/cloudeteer/m365-exporter/pkg/util"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/security"
	"github.com/prometheus/client_golang/prometheus"
)

const subsystem = "securescore"

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	maxScore *prometheus.Desc
	curScore *prometheus.Desc
}

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),
		maxScore: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "max"),
			"The maximum achievable secure score",
			[]string{},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		curScore: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "current"),
			"Currently achieved secure score",
			[]string{},
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

	ch <- c.maxScore

	ch <- c.curScore
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	// set a timeout for the request
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cfg := &security.SecureScoresRequestBuilderGetRequestConfiguration{
		QueryParameters: &security.SecureScoresRequestBuilderGetQueryParameters{
			Top: to.Ptr(int32(1)),
		},
	}

	scores, err := c.GraphClient().Security().SecureScores().Get(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get secure scores: %w", util.GetOdataError(err))
	}

	resultValues := scores.GetValue()

	metrics := make([]prometheus.Metric, 0, len(resultValues)*2)

	for _, score := range scores.GetValue() {
		if maxScore := score.GetMaxScore(); maxScore != nil {
			metrics = append(metrics, prometheus.MustNewConstMetric(
				c.maxScore,
				prometheus.GaugeValue,
				*maxScore,
			))
		}

		if currentScore := score.GetCurrentScore(); currentScore != nil {
			metrics = append(metrics, prometheus.MustNewConstMetric(
				c.curScore,
				prometheus.GaugeValue,
				*currentScore,
			))
		}
	}

	return metrics, nil
}
