package power_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/cloudeteer/m365-exporter/internal/testutil"
	"github.com/cloudeteer/m365-exporter/pkg/auth"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/power"
	"github.com/cloudeteer/m365-exporter/pkg/httpclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_ScrapeMetrics(t *testing.T) {
	t.Parallel()

	var (
		ok       bool
		tenantID string
	)

	if tenantID, ok = os.LookupEnv("AZURE_TENANT_ID"); !ok {
		t.Skip("no AZURE_TENANT_ID environment variable set")
	}

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// TODO: make this a singleton for all tests
	msGraphClient, azureCredential, err := auth.NewMSGraphClient(http.DefaultClient)
	require.NoError(t, err)

	httpClient := httpclient.New(prometheus.NewRegistry())
	httpClient.WithAzureCredential(azureCredential)

	collector := power.NewCollector(logger, tenantID, msGraphClient, httpClient.GetHTTPClient())

	// TODO: Go 1.24: Change to t.Context()
	metrics, err := collector.ScrapeMetrics(context.TODO())
	require.NoError(t, err)

	assert.NotEmpty(t, metrics)

	allMetrics, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)

	assert.NotEmpty(t, allMetrics)
	assert.Contains(t, allMetrics, "m365_power_capacity_rated_consumption")
	assert.Contains(t, allMetrics, "m365_power_capacity_entitlement_total")
	assert.Contains(t, allMetrics, "m365_power_capacity_licenses_paid_status_count")
	assert.Contains(t, allMetrics, "m365_power_capacity_licenses_trial_status_count")
}

func TestCollector_ScrapeMetrics_MockServer(t *testing.T) {
	t.Parallel()

	// Create mock response data
	mockResponse := power.TenantCapacities{
		TenantCapacities: []struct {
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
		}{
			{
				CapacityType:  "Database",
				CapacityUnits: "MB",
				TotalCapacity: 51200.0,
				MaxCapacity:   51200.0,
				Consumption: struct {
					Actual          float64   `json:"actual"`
					Rated           float64   `json:"rated"`
					ActualUpdatedOn time.Time `json:"actualUpdatedOn"`
					RatedUpdatedOn  time.Time `json:"ratedUpdatedOn"`
				}{
					Actual:          40000.0,
					Rated:           41424.554000000004,
					ActualUpdatedOn: time.Now(),
					RatedUpdatedOn:  time.Now(),
				},
				Status:           "Normal",
				OverflowCapacity: []interface{}{},
				CapacityEntitlements: []struct {
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
				}{
					{
						CapacityType:    "Database",
						CapacitySubType: "DatabaseBase",
						TotalCapacity:   10240.0,
						Licenses: []struct {
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
						}{
							{
								EntitlementCode: "MicrosoftFormsPro",
								DisplayName:     "Microsoft Forms Pro",
								ServicePlanId:   "test-plan-id",
								SkuId:           "98619618-9dc8-48c6-8f0c-741890ba5f93",
								Paid: struct {
									Enabled   int `json:"enabled"`
									Warning   int `json:"warning"`
									Suspended int `json:"suspended"`
								}{
									Enabled:   44,
									Warning:   0,
									Suspended: 0,
								},
								Trial: struct {
									Enabled   int `json:"enabled"`
									Warning   int `json:"warning"`
									Suspended int `json:"suspended"`
								}{
									Enabled:   0,
									Warning:   0,
									Suspended: 0,
								},
								TotalCapacity:     10240.0,
								NextLifecycleDate: time.Now().AddDate(1, 0, 0),
								CapabilityStatus:  "Enabled",
							},
						},
					},
				},
			},
		},
	}

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		err := json.NewEncoder(w).Encode(mockResponse)
		require.NoError(t, err)
	}))
	defer server.Close()

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a mock HTTP client that uses our test server
	client := &http.Client{
		Transport: &mockTransport{
			serverURL: server.URL,
		},
	}

	// Create collector with mock HTTP client
	collector := power.NewCollector(logger, "test-tenant", nil, client)

	// TODO: Go 1.24: Change to t.Context()
	metrics, err := collector.ScrapeMetrics(context.TODO())
	require.NoError(t, err)

	assert.NotEmpty(t, metrics)

	// Convert metrics to text for easier assertions
	allMetrics, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)

	assert.NotEmpty(t, allMetrics)

	// Check that all expected metrics are present
	assert.Contains(t, allMetrics, "m365_power_capacity_rated_consumption")
	assert.Contains(t, allMetrics, "m365_power_capacity_entitlement_total")
	assert.Contains(t, allMetrics, "m365_power_capacity_licenses_paid_status_count")
	assert.Contains(t, allMetrics, "m365_power_capacity_licenses_trial_status_count")

	// Check specific metric values
	assert.Contains(t, allMetrics, `m365_power_capacity_rated_consumption{capacityType="Database",capacityUnits="MB",tenant="test-tenant"} 41424.554000000004`)
	assert.Contains(t, allMetrics, `m365_power_capacity_entitlement_total{capacitySubType="DatabaseBase",capacityType="Database",tenant="test-tenant"} 10240`)
	assert.Contains(t, allMetrics, `m365_power_capacity_licenses_paid_status_count{capacitySubType="DatabaseBase",capacityType="Database",displayName="Microsoft Forms Pro",entitlementCode="MicrosoftFormsPro",skuId="98619618-9dc8-48c6-8f0c-741890ba5f93",status="enabled",tenant="test-tenant"} 44`)
	assert.Contains(t, allMetrics, `m365_power_capacity_licenses_trial_status_count{capacitySubType="DatabaseBase",capacityType="Database",displayName="Microsoft Forms Pro",entitlementCode="MicrosoftFormsPro",skuId="98619618-9dc8-48c6-8f0c-741890ba5f93",status="enabled",tenant="test-tenant"} 0`)
}

func TestCollector_ScrapeMetrics_HTTPError(t *testing.T) {
	t.Parallel()

	// Create mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a mock HTTP client that uses our test server
	client := &http.Client{
		Transport: &mockTransport{
			serverURL: server.URL,
		},
	}

	// Create collector with mock HTTP client
	collector := power.NewCollector(logger, "test-tenant", nil, client)

	// TODO: Go 1.24: Change to t.Context()
	metrics, err := collector.ScrapeMetrics(context.TODO())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code 500")
	assert.Empty(t, metrics)
}

func TestCollector_ScrapeMetrics_InvalidJSON(t *testing.T) {
	t.Parallel()

	// Create mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a mock HTTP client that uses our test server
	client := &http.Client{
		Transport: &mockTransport{
			serverURL: server.URL,
		},
	}

	// Create collector with mock HTTP client
	collector := power.NewCollector(logger, "test-tenant", nil, client)

	// TODO: Go 1.24: Change to t.Context()
	metrics, err := collector.ScrapeMetrics(context.TODO())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error unmarshalling response")
	assert.Empty(t, metrics)
}

func TestCollector_Describe(t *testing.T) {
	t.Parallel()

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	collector := power.NewCollector(logger, "test-tenant", nil, http.DefaultClient)

	ch := make(chan *prometheus.Desc, 10)
	go func() {
		collector.Describe(ch)
		close(ch)
	}()

	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}

	// Should have 4 custom metrics + 3 base collector metrics
	assert.Len(t, descs, 7)
}

func TestCollector_GetSubsystem(t *testing.T) {
	t.Parallel()

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	collector := power.NewCollector(logger, "test-tenant", nil, http.DefaultClient)

	assert.Equal(t, "power", collector.GetSubsystem())
}

// mockTransport is custom transport that redirects requests to our test server
type mockTransport struct {
	serverURL string
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect the request to our test server
	req.URL.Scheme = "http"
	req.URL.Host = req.URL.Host
	if t.serverURL != "" {
		// Parse the server URL to get host
		req.URL.Host = req.URL.Host
		// Replace with test server URL
		req.URL.Scheme = "http"
		// Extract host from serverURL
		newReq := req.Clone(req.Context())
		newReq.URL.Host = "127.0.0.1"
		if len(t.serverURL) > 7 { // "http://"
			newReq.URL.Host = t.serverURL[7:] // Remove "http://"
		}
		return http.DefaultTransport.RoundTrip(newReq)
	}
	return http.DefaultTransport.RoundTrip(req)
}
