package adsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/organization"
	"github.com/prometheus/client_golang/prometheus"
)

const subsystem = "adsync"

const (
	URLServiceADSyncError      = "https://management.azure.com/providers/Microsoft.ADHybridHealthService/services/%s/exporterrors/counts?api-version=2014-01-01"
	URLAllServicesADSyncErrors = "https://management.azure.com/providers/Microsoft.ADHybridHealthService/services?api-version=2014-01-01"
)

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	enabledDesc  *prometheus.Desc
	lastSyncDesc *prometheus.Desc
	errorDesc    *prometheus.Desc

	httpClient *http.Client
}

type entraIDServiceValue struct {
	ServiceName string `json:"serviceName"`
}

type entraIDServices struct {
	Value []entraIDServiceValue `json:"value"`
}

type azureError struct {
	Err struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (a azureError) Error() string {
	return fmt.Sprintf("%s %s", a.Err.Code, a.Err.Message)
}

type entraIDSyncError struct {
	ErrorBucket string `json:"errorBucket"`
	Count       int    `json:"count"`
	Truncated   bool   `json:"truncated"`
}

type entraIDServiceSyncErrors map[string][]entraIDSyncError

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient, httpClient *http.Client) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),

		enabledDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "on_premises_sync_enabled"),
			"status of azure ad connect synchronization",
			[]string{"organization"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		lastSyncDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "on_premises_last_sync_date_time"),
			"last Unix time of azure ad connect synchronization",
			[]string{"organization"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		errorDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "on_premises_sync_error"),
			"count of entra id connect synchronization errors",
			[]string{"sync_service", "error_bucket"},
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

	ch <- c.enabledDesc

	ch <- c.lastSyncDesc

	ch <- c.errorDesc
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	metrics := make([]prometheus.Metric, 0, 2)
	errorMetrics := make([]prometheus.Metric, 0, 2)

	queryParameters := organization.OrganizationRequestBuilderGetQueryParameters{
		Select: []string{"onPremisesLastSyncDateTime", "onPremisesSyncEnabled", "id"},
	}
	requestConfiguration := organization.OrganizationRequestBuilderGetRequestConfiguration{
		QueryParameters: &queryParameters,
	}

	result, err := c.GraphClient().Organization().Get(ctx, &requestConfiguration)
	if err != nil {
		return nil, fmt.Errorf("error getting organizations: %w", err)
	}

	for i, org := range result.GetValue() {
		if i != 0 {
			break
		} // WARN: there can be multiple Sync status! Depending on how many <Organizations> tied to the tenant

		azureAdSyncEnabledValue := 0
		if *org.GetOnPremisesSyncEnabled() {
			azureAdSyncEnabledValue = 1

			errorMetrics, err = c.scrapeErrors(ctx)
			if err != nil {
				return nil, fmt.Errorf("error scraping Azure AD Sync Errors: %w", err)
			}
		}

		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.enabledDesc,
			prometheus.GaugeValue,
			float64(azureAdSyncEnabledValue),
			*org.GetId(),
		))

		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.lastSyncDesc,
			prometheus.GaugeValue,
			float64(org.GetOnPremisesLastSyncDateTime().Unix()),
			*org.GetId(),
		))
	}

	return slices.Concat(metrics, errorMetrics), nil
}

func (c *Collector) scrapeErrors(ctx context.Context) ([]prometheus.Metric, error) {
	services, err := c.getSyncServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting Azure AD Sync Services: %w", err)
	}

	entraIDServiceSyncErrors := make(entraIDServiceSyncErrors)

	for _, service := range services.Value {
		syncErrors, err := c.getEntraServiceSyncErrors(ctx, service)
		if err != nil {
			return nil, fmt.Errorf("error getting errors: %w", err)
		}

		entraIDServiceSyncErrors[service.ServiceName] = syncErrors
	}

	metrics := make([]prometheus.Metric, 0, 2)

	for serviceName, syncErrors := range entraIDServiceSyncErrors {
		for _, syncError := range syncErrors {
			metrics = append(metrics, prometheus.MustNewConstMetric(
				c.errorDesc,
				prometheus.GaugeValue,
				float64(syncError.Count),
				serviceName,
				syncError.ErrorBucket,
			))
		}
	}

	return metrics, nil
}

func (c *Collector) getSyncServices(ctx context.Context) (entraIDServices, error) {
	var services entraIDServices

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, URLAllServicesADSyncErrors, nil)
	if err != nil {
		return services, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return services, fmt.Errorf("error sending request: %w", err)
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			c.logger.ErrorContext(ctx, "error closing response body", slog.Any("err", err))
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return services, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var azureError azureError

		err = json.Unmarshal(body, &azureError)
		if err != nil {
			return services, fmt.Errorf("error unmarshalling response (status %d): %s", resp.StatusCode, body)
		}

		return services, fmt.Errorf("unexpected status code %d: %w", resp.StatusCode, azureError)
	}

	err = json.Unmarshal(body, &services)
	if err != nil {
		return services, fmt.Errorf("error unmarshalling response: body %s, error %w", body, err)
	}

	return services, nil
}

func (c *Collector) getEntraServiceSyncErrors(ctx context.Context, service entraIDServiceValue) ([]entraIDSyncError, error) {
	url := fmt.Sprintf(URLServiceADSyncError, service.ServiceName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
		var azureError azureError

		err = json.Unmarshal(body, &azureError)
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling response (status %d): %s", resp.StatusCode, body)
		}

		return nil, fmt.Errorf("unexpected status code %d: %w", resp.StatusCode, azureError)
	}

	var entraIDSyncErrors []entraIDSyncError

	err = json.Unmarshal(body, &entraIDSyncErrors)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling response: body %s, error %w", body, err)
	}

	if entraIDSyncErrors == nil {
		return nil, errors.New("no entraID errors found")
	}

	return entraIDSyncErrors, nil
}
