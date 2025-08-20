package intune

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

	// VPP Token Status Values
	vppStatusUnknown               = 0.0
	vppStatusValid                 = 1.0
	vppStatusExpired               = 2.0
	vppStatusInvalid               = 3.0
	vppStatusAssignedToExternalMDM = 4.0

	// DEP Onboarding Settings API URL
	URLDepOnboardingSettings = "https://graph.microsoft.com/beta/deviceManagement/depOnboardingSettings"
)

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

// depOnboardingSetting represents a DEP onboarding setting from the Microsoft Graph API
type depOnboardingSetting struct {
	ID                      string    `json:"id"`
	AppleIdentifier         string    `json:"appleIdentifier"`
	TokenExpirationDateTime time.Time `json:"tokenExpirationDateTime"`
	TokenName               string    `json:"tokenName"`
}

// depOnboardingSettingsResponse represents the response from the DEP onboarding settings API
type depOnboardingSettingsResponse struct {
	Value []depOnboardingSetting `json:"value"`
}

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	complianceDesc *prometheus.Desc
	osDesc         *prometheus.Desc
	vppStatusDesc  *prometheus.Desc
	vppExpiryDesc  *prometheus.Desc
	depExpiryDesc  *prometheus.Desc

	httpClient *http.Client
}

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient, httpClient *http.Client) *Collector {
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
			"Status of Apple VPP tokens (0=unknown, 1=valid, 2=expired, 3=invalid, 4=assigned_to_external_mdm)",
			[]string{"appleId", "organizationName", "id"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		vppExpiryDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "vpp_expiry"),
			"Expiration timestamp of Apple VPP tokens in Unix timestamp",
			[]string{"appleId", "organizationName", "id"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		depExpiryDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "dep_token_expiry"),
			"Expiration timestamp of Apple DEP onboarding tokens in Unix timestamp",
			[]string{"appleIdentifier", "id"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),

		httpClient: httpClient,
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

	ch <- c.depExpiryDesc
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
		errs = append(errs, fmt.Errorf("error scraping apple vpp token metrics: %w", err))
	}

	depMetrics, err := c.scrapeDepOnboardingSettings(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("error scraping apple dep onboarding settings metrics: %w", err))
	}

	return slices.Concat(complianceMetrics, osMetrics, vppMetrics, depMetrics), errors.Join(errs...)
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

func (c *Collector) scrapeVppTokens(ctx context.Context) ([]prometheus.Metric, error) { //nolint:cyclop
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
		tokenId := vppToken.GetId()
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

		if tokenId == nil {
			tokenId = new(string)
			*tokenId = unknownValue
		}

		// Calculate status metric based on all possible states
		var statusValue float64

		if state != nil {
			switch *state {
			case models.UNKNOWN_VPPTOKENSTATE:
				statusValue = vppStatusUnknown
			case models.VALID_VPPTOKENSTATE:
				statusValue = vppStatusValid
			case models.EXPIRED_VPPTOKENSTATE:
				statusValue = vppStatusExpired
			case models.INVALID_VPPTOKENSTATE:
				statusValue = vppStatusInvalid
			case models.ASSIGNEDTOEXTERNALMDM_VPPTOKENSTATE:
				statusValue = vppStatusAssignedToExternalMDM
			}
		} else {
			statusValue = vppStatusUnknown
		}

		// Create status metric
		statusMetric := prometheus.MustNewConstMetric(
			c.vppStatusDesc,
			prometheus.GaugeValue,
			statusValue,
			*appleId, *organizationName, *tokenId,
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
			*appleId, *organizationName, *tokenId,
		)
		metrics = append(metrics, expiryMetric)

		return true
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate through VPP tokens: %w", util.GetOdataError(err))
	}

	return metrics, nil
}

func (c *Collector) scrapeDepOnboardingSettings(ctx context.Context) ([]prometheus.Metric, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, URLDepOnboardingSettings, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			c.logger.ErrorContext(ctx, "error closing response body", slog.Any("err", err))
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var depResponse depOnboardingSettingsResponse

	err = json.Unmarshal(body, &depResponse)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling response: body %s, error %w", string(body), err)
	}

	metrics := make([]prometheus.Metric, 0, len(depResponse.Value))

	for _, depSetting := range depResponse.Value {
		// Use Unix timestamp for token expiration
		expiryValue := float64(depSetting.TokenExpirationDateTime.Unix())

		// Create metric with appleIdentifier, id, and tenantId as labels
		metric := prometheus.MustNewConstMetric(
			c.depExpiryDesc,
			prometheus.GaugeValue,
			expiryValue,
			depSetting.AppleIdentifier,
			depSetting.ID,
		)
		metrics = append(metrics, metric)
	}

	return metrics, nil
}
