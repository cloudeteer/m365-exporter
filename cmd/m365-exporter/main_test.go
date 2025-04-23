package main_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
)

// MockGraphClient implements a minimal interface for testing.
type MockGraphClient struct{}

func (m *MockGraphClient) Organization() interface{} {
	return nil
}

func (m *MockGraphClient) Users() interface{} {
	return nil
}

func (m *MockGraphClient) Groups() interface{} {
	return nil
}

func (m *MockGraphClient) Sites() interface{} {
	return nil
}

func (m *MockGraphClient) Security() interface{} {
	return nil
}

func (m *MockGraphClient) DeviceManagement() interface{} {
	return nil
}

// setupTestCollectors is a test helper that sets up collectors with the given configuration.
func setupTestCollectors(ctx context.Context, logger *slog.Logger, reg *prometheus.Registry, tenantID string, msGraphClient *MockGraphClient, httpClient *http.Client) error {
	// Create a list of collectors with their configuration keys
	collectors := []struct {
		name     string
		enabled  bool
		interval time.Duration
	}{
		{"adsync", viper.GetBool("adsync.enabled"), time.Hour},
		{"exchange", viper.GetBool("exchange.enabled"), time.Hour},
		{"securescore", viper.GetBool("securescore.enabled"), time.Hour},
		{"license", viper.GetBool("license.enabled"), time.Hour},
		{"servicehealth", viper.GetBool("servicehealth.enabled"), time.Hour},
		{"intune", viper.GetBool("intune.enabled"), 3 * time.Hour},
		{"entraid", viper.GetBool("entraid.enabled"), 3 * time.Hour},
		{"sharepoint", viper.GetBool("sharepoint.enabled"), time.Hour},
		{"teams", viper.GetBool("teams.enabled"), 3 * time.Hour},
		{"onedrive", viper.GetBool("onedrive.enabled"), 3 * time.Hour},
	}

	// Register enabled collectors
	for _, c := range collectors {
		if !c.enabled {
			continue
		}

		// Create a simple collector that just reports its name
		collector := &testCollector{
			name: c.name,
		}

		if err := reg.Register(collector); err != nil {
			return err
		}
	}

	return nil
}

// testCollector is a simple collector implementation for testing.
type testCollector struct {
	name string
}

func (c *testCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc(
		"m365_test_collector",
		"Test collector",
		nil,
		prometheus.Labels{"collector": c.name},
	)
}

func (c *testCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.NewMetricWithTimestamp(
		time.Now(),
		prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "m365_test_collector",
			Help: "Test collector",
			ConstLabels: prometheus.Labels{
				"collector": c.name,
			},
		}),
	)
}

func (c *testCollector) GetSubsystem() string {
	return c.name
}

func TestCollectorConfiguration(t *testing.T) {
	// Test cases for different collector configurations
	testCases := []struct {
		name           string
		config         map[string]interface{}
		expectedCount  int
		collectorNames []string
	}{
		{
			name: "All collectors enabled",
			config: map[string]interface{}{
				"adsync.enabled":        true,
				"exchange.enabled":      true,
				"securescore.enabled":   true,
				"license.enabled":       true,
				"servicehealth.enabled": true,
				"intune.enabled":        true,
				"entraid.enabled":       true,
				"sharepoint.enabled":    true,
				"teams.enabled":         true,
				"onedrive.enabled":      true,
			},
			expectedCount: 10,
			collectorNames: []string{
				"adsync", "exchange", "securescore", "license", "servicehealth",
				"intune", "entraid", "sharepoint", "teams", "onedrive",
			},
		},
		{
			name: "Some collectors disabled",
			config: map[string]interface{}{
				"adsync.enabled":        true,
				"exchange.enabled":      false,
				"securescore.enabled":   true,
				"license.enabled":       false,
				"servicehealth.enabled": true,
				"intune.enabled":        false,
				"entraid.enabled":       true,
				"sharepoint.enabled":    false,
				"teams.enabled":         true,
				"onedrive.enabled":      false,
			},
			expectedCount: 5,
			collectorNames: []string{
				"adsync", "securescore", "servicehealth", "entraid", "teams",
			},
		},
		{
			name: "All collectors disabled",
			config: map[string]interface{}{
				"adsync.enabled":        false,
				"exchange.enabled":      false,
				"securescore.enabled":   false,
				"license.enabled":       false,
				"servicehealth.enabled": false,
				"intune.enabled":        false,
				"entraid.enabled":       false,
				"sharepoint.enabled":    false,
				"teams.enabled":         false,
				"onedrive.enabled":      false,
			},
			expectedCount:  0,
			collectorNames: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new registry for each test case
			reg := prometheus.NewRegistry()

			// Create a test logger
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))

			// Create a test context
			ctx := context.Background()

			// Reset viper for each test case
			viper.Reset()

			// Set test configuration
			for k, v := range tc.config {
				viper.Set(k, v)
			}

			// Create mock clients
			mockGraphClient := &MockGraphClient{}
			mockHTTPClient := &http.Client{}

			// Setup metrics collectors
			err := setupTestCollectors(ctx, logger, reg, "test-tenant", mockGraphClient, mockHTTPClient)
			if err != nil {
				t.Fatalf("Failed to setup metrics collectors: %v", err)
			}

			// Get all registered collectors
			collectorCount := 0
			collectorNames := make(map[string]bool)

			// Get all metrics from the registry
			metricFamilies, err := reg.Gather()
			if err != nil {
				t.Fatalf("Failed to gather metrics: %v", err)
			}

			// Process each metric family
			for _, mf := range metricFamilies {
				for _, m := range mf.GetMetric() {
					// Check if this is a test collector metric
					for _, l := range m.GetLabel() {
						if l.GetName() == "collector" {
							collectorName := l.GetValue()
							if !collectorNames[collectorName] {
								collectorNames[collectorName] = true
								collectorCount++
							}

							break
						}
					}
				}
			}

			// Verify the number of registered collectors
			if collectorCount != tc.expectedCount {
				t.Errorf("Expected %d collectors, got %d", tc.expectedCount, collectorCount)
			}

			// Verify collector names
			for _, name := range tc.collectorNames {
				if !collectorNames[name] {
					t.Errorf("Expected to find collector %s, but it was not registered", name)
				}
			}
		})
	}
}
