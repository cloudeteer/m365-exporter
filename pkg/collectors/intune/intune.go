package intune

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/cloudeteer/m365-exporter/pkg/util"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/prometheus/client_golang/prometheus"
)

const subsystem = "intune"

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	complianceDesc *prometheus.Desc
}

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),

		complianceDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "device_compliance"),
			"Compliance of devices managed by Intune",
			[]string{"type"},
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

	ch <- c.complianceDesc
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	metrics := make([]prometheus.Metric, 0, 10)

	all, err := c.GraphClient().DeviceManagement().ManagedDeviceOverview().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get managed device overview: %w", util.GetOdataError(err))
	}

	metrics = append(metrics, prometheus.MustNewConstMetric(
		c.complianceDesc,
		prometheus.GaugeValue,
		float64(*all.GetEnrolledDeviceCount()),
		"all",
	))

	detailed, err := c.GraphClient().DeviceManagement().DeviceCompliancePolicyDeviceStateSummary().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("there was a problem getting device state summary: %w", util.GetOdataError(err))
	}

	for label, deviceCount := range map[string]float64{
		"compliant":     float64(*detailed.GetCompliantDeviceCount()),
		"noncompliant":  float64(*detailed.GetNonCompliantDeviceCount()),
		"unknown":       float64(*detailed.GetUnknownDeviceCount()),
		"graceperiod":   float64(*detailed.GetInGracePeriodCount()),
		"remediated":    float64(*detailed.GetRemediatedDeviceCount()),
		"conflict":      float64(*detailed.GetConflictDeviceCount()),
		"error":         float64(*detailed.GetErrorDeviceCount()),
		"notapplicable": float64(*detailed.GetNotApplicableDeviceCount()),
	} {
		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.complianceDesc,
			prometheus.GaugeValue,
			deviceCount,
			label,
		))
	}

	return metrics, nil
}
