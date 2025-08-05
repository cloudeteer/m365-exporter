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
	unknownValue          = "unknown"
)

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	complianceDesc *prometheus.Desc
	osDesc         *prometheus.Desc
	vppStatusDesc  *prometheus.Desc
	vppExpiryDesc  *prometheus.Desc
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
		vppStatusDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "vpp_status"),
			"Status of VPP tokens (1 for valid, 0 for other states)",
			[]string{"appleId", "organizationName"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		vppExpiryDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "vpp_expiry"),
			"Expiration timestamp of VPP tokens in Unix timestamp",
			[]string{"appleId", "organizationName"},
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

	ch <- c.osDesc

	ch <- c.vppStatusDesc

	ch <- c.vppExpiryDesc
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

	vppMetrics, err := c.scrapeVppTokens(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("error scraping vpp token metrics: %w", err))
	}

	return slices.Concat(complianceMetrics, osMetrics, vppMetrics), errors.Join(errs...)
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
			*osName = unknownValue
		}

		if osVersion == nil || *osVersion == "" {
			*osVersion = unknownValue
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

func (c *Collector) scrapeVppTokens(ctx context.Context) ([]prometheus.Metric, error) {
	vppTokens, err := c.GraphClient().DeviceAppManagement().VppTokens().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get VPP tokens: %w", util.GetOdataError(err))
	}

	vIterator, err := graphcore.NewPageIterator[*models.VppToken](
		vppTokens,
		c.GraphClient().GetAdapter(),
		models.CreateVppTokenCollectionResponseFromDiscriminatorValue,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create VPP token iterator: %w", util.GetOdataError(err))
	}

	metrics := make([]prometheus.Metric, 0)

	err = vIterator.Iterate(ctx, func(vppToken *models.VppToken) bool {
		// Get required fields
		appleId := vppToken.GetAppleId()
		organizationName := vppToken.GetOrganizationName()
		state := vppToken.GetState()
		expirationDateTime := vppToken.GetExpirationDateTime()

		// Handle nil values
		if appleId == nil {
			appleId = new(string)
			*appleId = unknownValue
		}

		if organizationName == nil {
			organizationName = new(string)
			*organizationName = unknownValue
		}

		// Calculate status metric (1 for valid, 0 for other states)
		var statusValue float64
		if state != nil && *state == models.VppTokenState(1) { // VppTokenState(1) is "valid"
			statusValue = 1.0
		} else {
			statusValue = 0.0
		}

		// Create status metric
		statusMetric := prometheus.MustNewConstMetric(
			c.vppStatusDesc,
			prometheus.GaugeValue,
			statusValue,
			*appleId, *organizationName,
		)
		metrics = append(metrics, statusMetric)

		// Calculate expiry metric (Unix timestamp)
		var expiryValue float64
		if expirationDateTime != nil {
			expiryValue = float64(expirationDateTime.Unix())
		} else {
			// If no expiration date, use 0
			expiryValue = 0.0
		}

		// Create expiry metric
		expiryMetric := prometheus.MustNewConstMetric(
			c.vppExpiryDesc,
			prometheus.GaugeValue,
			expiryValue,
			*appleId, *organizationName,
		)
		metrics = append(metrics, expiryMetric)

		return true
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate through VPP tokens: %w", util.GetOdataError(err))
	}

	return metrics, nil
}
