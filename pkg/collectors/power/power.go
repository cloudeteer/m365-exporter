package power

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/prometheus/client_golang/prometheus"
)

const subsystem = "power"

const (
	URLPowerPlatformCapacities = "https://api.powerplatform.com/licensing/tenantCapacity?api-version=2022-03-01-preview"
)

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	capacityRatedConsumptionDesc *prometheus.Desc
	capacityEntitlementTotalDesc *prometheus.Desc
	licensesPaidStatusCountDesc  *prometheus.Desc
	licensesTrialStatusCountDesc *prometheus.Desc

	httpClient *http.Client
}

type TenantCapacities struct {
	TenantCapacities []struct {
		CapacityType  string  `json:"capacityType"`
		CapacityUnits string  `json:"capacityUnits"`
		TotalCapacity float64 `json:"totalCapacity"`
		MaxCapacity   float64 `json:"maxCapacity"`
		Consumption   struct {
			Actual          float64   `json:"actual"`
			Rated           float64   `json:"rated"`
			ActualUpdatedOn time.Time `json:"actualUpdatedOn"`
			RatedUpdatedOn  time.Time `json:"ratedUpdatedOn"`
		} `json:"consumption"`
		Status               string        `json:"status"`
		OverflowCapacity     []interface{} `json:"overflowCapacity"`
		CapacityEntitlements []struct {
			CapacityType         string    `json:"capacityType"`
			CapacitySubType      string    `json:"capacitySubType"`
			TotalCapacity        float64   `json:"totalCapacity"`
			MaxNextLifecycleDate time.Time `json:"maxNextLifecycleDate,omitempty"`
			Licenses             []struct {
				EntitlementCode string `json:"entitlementCode"`
				DisplayName     string `json:"displayName"`
				ServicePlanId   string `json:"servicePlanId"`
				SkuId           string `json:"skuId"`
				Paid            struct {
					Enabled   int `json:"enabled"`
					Warning   int `json:"warning"`
					Suspended int `json:"suspended"`
				} `json:"paid"`
				Trial struct {
					Enabled   int `json:"enabled"`
					Warning   int `json:"warning"`
					Suspended int `json:"suspended"`
				} `json:"trial"`
				TotalCapacity     float64   `json:"totalCapacity"`
				NextLifecycleDate time.Time `json:"nextLifecycleDate"`
				CapabilityStatus  string    `json:"capabilityStatus"`
			} `json:"licenses"`
		} `json:"capacityEntitlements"`
	} `json:"tenantCapacities"`
}

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient, httpClient *http.Client) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),

		capacityRatedConsumptionDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "capacity_rated_consumption"),
			"Rated consumption of M365 Power Capacity in MB",
			[]string{"capacityType", "capacityUnits"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		capacityEntitlementTotalDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "capacity_entitlement_total"),
			"Total capacity provided by a specific entitlement in MB",
			[]string{"capacityType", "capacitySubType"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		licensesPaidStatusCountDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "capacity_licenses_paid_status_count"),
			"The number of paid licenses by status",
			[]string{"entitlementCode", "displayName", "skuId", "status", "capacityType", "capacitySubType"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		licensesTrialStatusCountDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "capacity_licenses_trial_status_count"),
			"The number of trial licenses by status",
			[]string{"entitlementCode", "displayName", "skuId", "status", "capacityType", "capacitySubType"},
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

	ch <- c.capacityRatedConsumptionDesc

	ch <- c.capacityEntitlementTotalDesc

	ch <- c.licensesPaidStatusCountDesc

	ch <- c.licensesTrialStatusCountDesc
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	tenantCapacities, err := c.getTenantCapacities(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting tenant capacities: %w", err)
	}

	metrics := make([]prometheus.Metric, 0)

	for _, capacity := range tenantCapacities.TenantCapacities {
		// Metric 1: Rated consumption
		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.capacityRatedConsumptionDesc,
			prometheus.GaugeValue,
			capacity.Consumption.Rated,
			capacity.CapacityType,
			capacity.CapacityUnits,
		))

		// Metric 2: Capacity entitlements
		for _, entitlement := range capacity.CapacityEntitlements {
			metrics = append(metrics, prometheus.MustNewConstMetric(
				c.capacityEntitlementTotalDesc,
				prometheus.GaugeValue,
				entitlement.TotalCapacity,
				entitlement.CapacityType,
				entitlement.CapacitySubType,
			))

			// Metrics 3 & 4: License counts (paid and trial)
			for _, license := range entitlement.Licenses {
				// Paid licenses
				metrics = append(metrics, prometheus.MustNewConstMetric(
					c.licensesPaidStatusCountDesc,
					prometheus.GaugeValue,
					float64(license.Paid.Enabled),
					license.EntitlementCode,
					license.DisplayName,
					license.SkuId,
					"enabled",
					entitlement.CapacityType,
					entitlement.CapacitySubType,
				))
				metrics = append(metrics, prometheus.MustNewConstMetric(
					c.licensesPaidStatusCountDesc,
					prometheus.GaugeValue,
					float64(license.Paid.Warning),
					license.EntitlementCode,
					license.DisplayName,
					license.SkuId,
					"warning",
					entitlement.CapacityType,
					entitlement.CapacitySubType,
				))
				metrics = append(metrics, prometheus.MustNewConstMetric(
					c.licensesPaidStatusCountDesc,
					prometheus.GaugeValue,
					float64(license.Paid.Suspended),
					license.EntitlementCode,
					license.DisplayName,
					license.SkuId,
					"suspended",
					entitlement.CapacityType,
					entitlement.CapacitySubType,
				))

				// Trial licenses
				metrics = append(metrics, prometheus.MustNewConstMetric(
					c.licensesTrialStatusCountDesc,
					prometheus.GaugeValue,
					float64(license.Trial.Enabled),
					license.EntitlementCode,
					license.DisplayName,
					license.SkuId,
					"enabled",
					entitlement.CapacityType,
					entitlement.CapacitySubType,
				))
				metrics = append(metrics, prometheus.MustNewConstMetric(
					c.licensesTrialStatusCountDesc,
					prometheus.GaugeValue,
					float64(license.Trial.Warning),
					license.EntitlementCode,
					license.DisplayName,
					license.SkuId,
					"warning",
					entitlement.CapacityType,
					entitlement.CapacitySubType,
				))
				metrics = append(metrics, prometheus.MustNewConstMetric(
					c.licensesTrialStatusCountDesc,
					prometheus.GaugeValue,
					float64(license.Trial.Suspended),
					license.EntitlementCode,
					license.DisplayName,
					license.SkuId,
					"suspended",
					entitlement.CapacityType,
					entitlement.CapacitySubType,
				))
			}
		}
	}

	return metrics, nil
}

func (c *Collector) getTenantCapacities(ctx context.Context) (TenantCapacities, error) {
	var tenantCapacities TenantCapacities

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, URLPowerPlatformCapacities, nil)
	if err != nil {
		return tenantCapacities, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json;odata.metadata=minimal")
	req.Header.Set("OData-Version", "4.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return tenantCapacities, fmt.Errorf("error sending request: %w", err)
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			c.logger.ErrorContext(ctx, "error closing response body", slog.Any("err", err))
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return tenantCapacities, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return tenantCapacities, fmt.Errorf(
			"unexpected status code %d: %s. this can be due to missing permissions in Power Platform. Please revisit documentation",
			resp.StatusCode, string(body))
	}

	err = json.Unmarshal(body, &tenantCapacities)
	if err != nil {
		return tenantCapacities, fmt.Errorf("error unmarshalling response: body %s, error %w", body, err)
	}

	return tenantCapacities, nil
}
