package intune

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
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
	labelTenant           = "tenant"
	labelAppleID          = "appleId"

	// VPP Token Status Values
	vppStatusUnknown               = 0.0
	vppStatusValid                 = 1.0
	vppStatusExpired               = 2.0
	vppStatusInvalid               = 3.0
	vppStatusAssignedToExternalMDM = 4.0

	// URLDepOnboardingSettings DEP Onboarding Settings API URL
	URLDepOnboardingSettings = "https://graph.microsoft.com/beta/deviceManagement/depOnboardingSettings"

	// URLDeviceConfigurations Device Configuration Profiles API URL
	URLDeviceConfigurations = "https://graph.microsoft.com/beta/deviceManagement/deviceConfigurations"
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

// Settings holds configuration for the Intune collector.
type Settings struct {
	PerProfileConfiguration       bool
	PerProfileConfigurationFilter []string
}

type configurationProfile struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type configurationProfileListResponse struct {
	Value    []configurationProfile `json:"value"`
	NextLink string                 `json:"@odata.nextLink"`
}

type deviceConfigurationStatusOverview struct {
	ID                   string `json:"id"`
	SuccessCount         int    `json:"successCount"`
	FailedCount          int    `json:"failedCount"`
	ErrorCount           int    `json:"errorCount"`
	PendingCount         int    `json:"pendingCount"`
	NotApplicableCount   int    `json:"notApplicableCount"`
	ConfigurationVersion int    `json:"configurationVersion"`
	LastUpdateDateTime   string `json:"lastUpdateDateTime"`
}

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	complianceDesc              *prometheus.Desc
	osDesc                      *prometheus.Desc
	vppStatusDesc               *prometheus.Desc
	vppExpiryDesc               *prometheus.Desc
	depExpiryDesc               *prometheus.Desc
	apnExpiryDesc               *prometheus.Desc
	perProfileConfigurationDesc *prometheus.Desc

	httpClient        *http.Client
	perProfileEnabled bool
	profileFilter     []string
}

func NewCollector(
	logger *slog.Logger,
	tenant string,
	msGraphClient *msgraphsdk.GraphServiceClient,
	httpClient *http.Client,
	settings Settings,
) *Collector {
	collectorLogger := logger.With(slog.String("collector", subsystem))

	// Validate glob patterns at startup
	for _, pattern := range settings.PerProfileConfigurationFilter {
		_, err := path.Match(pattern, "test")
		if err != nil {
			collectorLogger.Warn("invalid glob pattern in perProfileConfigurationFilter, will be skipped during filtering",
				slog.String("pattern", pattern),
				slog.Any("err", err),
			)
		}
	}

	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        collectorLogger,

		complianceDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "device_compliance"),
			"Compliance of devices managed by Intune",
			[]string{"type"},
			prometheus.Labels{
				labelTenant: tenant,
			},
		),
		osDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "device_count"),
			"Device information of devices managed by Intune",
			[]string{"os_name", "os_version"},
			prometheus.Labels{
				labelTenant: tenant,
			},
		),
		vppStatusDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "vpp_status"),
			"Status of Apple VPP tokens (0=unknown, 1=valid, 2=expired, 3=invalid, 4=assigned_to_external_mdm)",
			[]string{labelAppleID, "organizationName", "id"},
			prometheus.Labels{
				labelTenant: tenant,
			},
		),
		vppExpiryDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "vpp_expiry"),
			"Expiration timestamp of Apple VPP tokens in Unix timestamp",
			[]string{labelAppleID, "organizationName", "id"},
			prometheus.Labels{
				labelTenant: tenant,
			},
		),
		depExpiryDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "dep_token_expiry"),
			"Expiration timestamp of Apple DEP onboarding tokens in Unix timestamp",
			[]string{labelAppleID, "id"},
			prometheus.Labels{
				labelTenant: tenant,
			},
		),
		apnExpiryDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "apn_expiry"),
			"Expiration timestamp of Apple Push Notification Certificate in Unix timestamp",
			[]string{labelAppleID, "topicIdentifier", "id"},
			prometheus.Labels{
				labelTenant: tenant,
			},
		),
		perProfileConfigurationDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "device_configuration_overview"),
			"Per-profile device configuration status aggregate counts",
			[]string{"profile_name", "status"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),

		httpClient:        httpClient,
		perProfileEnabled: settings.PerProfileConfiguration,
		profileFilter:     toLowerSlice(settings.PerProfileConfigurationFilter),
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

	ch <- c.apnExpiryDesc

	if c.perProfileEnabled {
		ch <- c.perProfileConfigurationDesc
	}
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

	apnMetrics, err := c.scrapeApplePushNotificationCertificate(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("error scraping apple push notification certificate metrics: %w", err))
	}

	var perProfileMetrics []prometheus.Metric

	if c.perProfileEnabled {
		perProfileMetrics, err = c.scrapePerProfileConfiguration(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("error scraping per-profile configuration metrics: %w", err))
		}
	}

	return slices.Concat(complianceMetrics, osMetrics, vppMetrics, depMetrics, apnMetrics, perProfileMetrics), errors.Join(errs...)
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
	var depResponse depOnboardingSettingsResponse

	err := c.getJSON(ctx, URLDepOnboardingSettings, &depResponse)
	if err != nil {
		return nil, fmt.Errorf("error fetching DEP onboarding settings: %w", err)
	}

	metrics := make([]prometheus.Metric, 0, len(depResponse.Value))

	for _, depSetting := range depResponse.Value {
		// Use Unix timestamp for token expiration
		expiryValue := float64(depSetting.TokenExpirationDateTime.Unix())

		// Create metric with appleId, id, and tenantId as labels
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

func (c *Collector) scrapeApplePushNotificationCertificate(ctx context.Context) ([]prometheus.Metric, error) {
	apnCertificate, err := c.GraphClient().DeviceManagement().ApplePushNotificationCertificate().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Apple Push Notification Certificate: %w", util.GetOdataError(err))
	}

	metrics := make([]prometheus.Metric, 0, 1)

	// Get required fields
	appleId := apnCertificate.GetAppleIdentifier()
	topicIdentifier := apnCertificate.GetTopicIdentifier()
	certificateId := apnCertificate.GetId()
	expirationDateTime := apnCertificate.GetExpirationDateTime()

	// Handle nil values
	if appleId == nil {
		appleId = new(string)
		*appleId = unknownValue
	}

	if topicIdentifier == nil {
		topicIdentifier = new(string)
		*topicIdentifier = unknownValue
	}

	if certificateId == nil {
		certificateId = new(string)
		*certificateId = unknownValue
	}

	// Calculate expiry metric (Unix timestamp)
	var expiryValue float64
	if expirationDateTime != nil {
		expiryValue = float64(expirationDateTime.Unix())
	} else {
		// If no expiration date, use 0
		expiryValue = 0.0
	}

	// Create metric with appleId, topicIdentifier, and id as labels
	metric := prometheus.MustNewConstMetric(
		c.apnExpiryDesc,
		prometheus.GaugeValue,
		expiryValue,
		*appleId,
		*topicIdentifier,
		*certificateId,
	)
	metrics = append(metrics, metric)

	return metrics, nil
}

func (c *Collector) getJSON(ctx context.Context, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			c.logger.ErrorContext(ctx, "error closing response body", slog.Any("err", err))
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	err = json.Unmarshal(body, target)
	if err != nil {
		return fmt.Errorf("error unmarshalling response: body %s, error %w", string(body), err)
	}

	return nil
}

func (c *Collector) fetchAllConfigurationProfiles(ctx context.Context) ([]configurationProfile, error) {
	all := make([]configurationProfile, 0, 50)

	url := URLDeviceConfigurations + "?$select=id,displayName"

	for url != "" {
		var resp configurationProfileListResponse

		err := c.getJSON(ctx, url, &resp)
		if err != nil {
			return nil, fmt.Errorf("error fetching configuration profiles: %w", err)
		}

		all = append(all, resp.Value...)
		url = resp.NextLink
	}

	return all, nil
}

func (c *Collector) fetchProfileStatusOverview(ctx context.Context, profileID string) (*deviceConfigurationStatusOverview, error) {
	url := URLDeviceConfigurations + "/" + profileID + "/deviceStatusOverview"

	var overview deviceConfigurationStatusOverview

	err := c.getJSON(ctx, url, &overview)
	if err != nil {
		return nil, fmt.Errorf("error fetching status overview for profile %s: %w", profileID, err)
	}

	return &overview, nil
}

func (c *Collector) scrapePerProfileConfiguration(ctx context.Context) ([]prometheus.Metric, error) {
	start := time.Now()

	profiles, err := c.fetchAllConfigurationProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("error fetching configuration profiles: %w", err)
	}

	// Pre-allocate for 5 metrics per profile (5 status types)
	metrics := make([]prometheus.Metric, 0, len(profiles)*5)

	for _, profile := range profiles {
		profileName := profile.DisplayName

		if profileName == "" {
			profileName = unknownValue
		}

		if !c.matchesProfileFilter(profileName) {
			continue
		}

		overview, err := c.fetchProfileStatusOverview(ctx, profile.ID)
		if err != nil {
			c.logger.ErrorContext(ctx, "error fetching status overview for profile, skipping",
				slog.String("profile_name", profileName),
				slog.String("profile_id", profile.ID),
				slog.Any("err", err),
			)

			continue
		}

		// Emit one metric per status type with count value
		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.perProfileConfigurationDesc,
			prometheus.GaugeValue,
			float64(overview.SuccessCount),
			profileName, "success",
		))
		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.perProfileConfigurationDesc,
			prometheus.GaugeValue,
			float64(overview.FailedCount),
			profileName, "failed",
		))
		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.perProfileConfigurationDesc,
			prometheus.GaugeValue,
			float64(overview.ErrorCount),
			profileName, "error",
		))
		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.perProfileConfigurationDesc,
			prometheus.GaugeValue,
			float64(overview.PendingCount),
			profileName, "pending",
		))
		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.perProfileConfigurationDesc,
			prometheus.GaugeValue,
			float64(overview.NotApplicableCount),
			profileName, "notApplicable",
		))
	}

	duration := time.Since(start)

	c.logger.InfoContext(ctx, "scraped per-profile configuration metrics",
		slog.Int("profiles", len(profiles)),
		slog.Int("metrics", len(metrics)),
		slog.Duration("duration", duration),
	)

	return metrics, nil
}

func (c *Collector) matchesProfileFilter(profileName string) bool {
	if len(c.profileFilter) == 0 {
		return true
	}

	lowerName := strings.ToLower(profileName)

	for _, pattern := range c.profileFilter {
		matched, err := path.Match(pattern, lowerName)
		if err != nil {
			c.logger.Warn("invalid glob pattern in profile filter, skipping",
				slog.String("pattern", pattern),
				slog.Any("err", err),
			)

			continue
		}

		if matched {
			return true
		}
	}

	return false
}

func toLowerSlice(ss []string) []string {
	out := make([]string, len(ss))

	for i, s := range ss {
		out[i] = strings.ToLower(s)
	}

	return out
}
