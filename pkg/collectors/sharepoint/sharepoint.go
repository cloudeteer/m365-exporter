package sharepoint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/sites"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cast"
)

const subsystem = "sharepoint"

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	sharepointDesc *prometheus.Desc

	httpClient *http.Client
}

type sharepointError struct {
	ErrorDescription string `json:"error_description"`
}

func (a sharepointError) Error() string {
	return a.ErrorDescription
}

type sharepointResponse struct {
	Value []struct {
		GeoAllocatedStorageMB   int    `json:"GeoAllocatedStorageMB"`
		GeoAvailableStorageMB   int    `json:"GeoAvailableStorageMB"`
		GeoLocation             string `json:"GeoLocation"`
		GeoUsedArchiveStorageMB int    `json:"GeoUsedArchiveStorageMB"`
		GeoUsedStorageMB        int    `json:"GeoUsedStorageMB"`
		QuotaType               int    `json:"QuotaType"`
		TenantStorageMB         int    `json:"TenantStorageMB"`
	} `json:"value"`
}

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient, httpClient *http.Client) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),

		sharepointDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "usage_info"),
			"Sharepoint metrics",
			[]string{"name", "type"},
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

	ch <- c.sharepointDesc
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	errs := make([]error, 0)
	metrics := make([]prometheus.Metric, 0, 1)

	sharepointList, sharepointListerr := c.findSharepoints(ctx)
	if sharepointListerr != nil {
		errs = append(errs, fmt.Errorf("error listing sharepoints: %w", sharepointListerr))
	}

	for _, sharepoint := range sharepointList {
		sharepointResponse, err := c.getSharepointMetrics(ctx, sharepoint)
		if err != nil {
			errs = append(errs, fmt.Errorf("error getting metrics from sharepoints: %w", err))

			continue
		}

		metrics = append(metrics,
			prometheus.MustNewConstMetric(
				c.sharepointDesc,
				prometheus.GaugeValue,
				float64(sharepointResponse.Value[0].GeoAllocatedStorageMB),
				sharepoint,
				"GeoAllocatedStorageMB",
			),
			prometheus.MustNewConstMetric(
				c.sharepointDesc,
				prometheus.GaugeValue,
				float64(sharepointResponse.Value[0].GeoAvailableStorageMB),
				sharepoint,
				"GeoAvailableStorageMB",
			),
			prometheus.MustNewConstMetric(
				c.sharepointDesc,
				prometheus.GaugeValue,
				float64(sharepointResponse.Value[0].GeoUsedArchiveStorageMB),
				sharepoint,
				"GeoUsedArchiveStorageMB",
			),
			prometheus.MustNewConstMetric(
				c.sharepointDesc,
				prometheus.GaugeValue,
				float64(sharepointResponse.Value[0].GeoUsedStorageMB),
				sharepoint,
				"GeoUsedStorageMB",
			),
			prometheus.MustNewConstMetric(
				c.sharepointDesc,
				prometheus.GaugeValue,
				float64(sharepointResponse.Value[0].QuotaType),
				sharepoint,
				"QuotaType",
			),
			prometheus.MustNewConstMetric(
				c.sharepointDesc,
				prometheus.GaugeValue,
				float64(sharepointResponse.Value[0].TenantStorageMB),
				sharepoint,
				"TenantStorageMB",
			),
		)
	}

	return metrics, errors.Join(errs...)
}

func (c *Collector) findSharepoints(ctx context.Context) ([]string, error) {
	// https://learn.microsoft.com/en-gb/graph/api/site-list?view=graph-rest-1.0&tabs=http
	filter := "siteCollection/root ne null"
	query := sites.SitesRequestBuilderGetQueryParameters{
		Select: []string{"siteCollection"},
		Filter: &filter,
	}
	config := sites.SitesRequestBuilderGetRequestConfiguration{
		QueryParameters: &query,
	}

	sharepointSites, err := c.GraphClient().Sites().Get(ctx, &config)
	if err != nil {
		return nil, fmt.Errorf("error fetching sharepoint sites: %w", err)
	}

	sharepoints := sharepointSites.GetValue()
	sharepointList := make([]string, 1)

	for count := range sharepoints {
		result := sharepoints[count].GetSiteCollection().GetHostname()
		hostname := strings.Split(cast.ToString(result), ".sharepoint.com")
		sharepointList[0] = hostname[0]
	}

	return sharepointList, nil
}

func (c *Collector) getSharepointMetrics(ctx context.Context, sharepoint string) (sharepointResponse, error) {
	var sharepointResponse sharepointResponse

	sharepointAddress := fmt.Sprintf("https://%s-admin.sharepoint.com/_api/StorageQuotas()?api-version=1.3.2", sharepoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sharepointAddress, nil)
	if err != nil {
		return sharepointResponse, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("odata-version", "4.0")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return sharepointResponse, fmt.Errorf("error sending request: %w", err)
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			c.logger.ErrorContext(ctx, "error closing response body", slog.Any("err", err))
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return sharepointResponse, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var sharepointError sharepointError

		err = json.Unmarshal(body, &sharepointError)
		if err != nil {
			return sharepointResponse, fmt.Errorf("error unmarshalling response (status %d): %s", resp.StatusCode, body)
		}

		return sharepointResponse, fmt.Errorf("unexpected status code %d: %w", resp.StatusCode, sharepointError)
	}

	err = json.Unmarshal(body, &sharepointResponse)
	if err != nil {
		return sharepointResponse, fmt.Errorf("error unmarshalling response: body %s, error %w", body, err)
	}

	return sharepointResponse, nil
}
