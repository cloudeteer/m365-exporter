package intune

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/cloudeteer/m365-exporter/pkg/util"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	subsystem             = "intune"
	osIdentifierSeparator = "___"
)

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	complianceDesc *prometheus.Desc
	osDesc         *prometheus.Desc
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
		osDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "device_count"),
			"Device information of devices managed by Intune",
			[]string{"os_name", "os_version"},
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
	errs := make([]error, 0)

	complianceMetrics, err := c.scrapeCompliance(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("error scraping compliance metrics: %w", err))
	}

	osMetrics, err := c.scrapeDevices(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("error scraping os metrics: %w", err))
	}

	return slices.Concat(complianceMetrics, osMetrics), errors.Join(errs...)
}

func (c *Collector) scrapeCompliance(ctx context.Context) ([]prometheus.Metric, error) {
	metrics := make([]prometheus.Metric, 0, 7)

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

func (c *Collector) scrapeDevices(ctx context.Context) ([]prometheus.Metric, error) {
	all, err := c.GraphClient().DeviceManagement().ManagedDevices().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get managed device overview: %w", util.GetOdataError(err))
	}

	dIterator, err := graphcore.NewPageIterator[*models.ManagedDevice](
		all,
		c.GraphClient().GetAdapter(),
		models.CreateManagedDeviceCollectionResponseFromDiscriminatorValue,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create device iterator: %w", util.GetOdataError(err))
	}

	metrics, err := c.iterateThroughDevices(ctx, dIterator)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

func (c *Collector) iterateThroughDevices(ctx context.Context, dIterator *graphcore.PageIterator[*models.ManagedDevice]) ([]prometheus.Metric, error) {
	osIdentifiers := make(map[string]float64)

	err := dIterator.Iterate(ctx, func(device *models.ManagedDevice) bool {
		osName := device.GetOperatingSystem()
		osVersion := device.GetOsVersion()

		if osName == nil || *osName == "" {
			*osName = "unknown"
		}

		if osVersion == nil || *osVersion == "" {
			*osVersion = "unknown"
		}

		osIdentifier := *osName + osIdentifierSeparator + *osVersion

		// map keys are created on the fly
		osIdentifiers[osIdentifier]++

		return true
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate through devices: %w", util.GetOdataError(err))
	}

	metrics := make([]prometheus.Metric, 0, len(osIdentifiers))

	for osIdentifier, count := range osIdentifiers {
		osIdentifierParts := strings.Split(osIdentifier, osIdentifierSeparator)

		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.osDesc,
			prometheus.GaugeValue,
			count,
			osIdentifierParts[0], osIdentifierParts[1],
		))
	}

	return metrics, nil
}
