package intune

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/cloudeteer/m365-exporter/internal/testutil"
	"github.com/cloudeteer/m365-exporter/pkg/auth"
	"github.com/cloudeteer/m365-exporter/pkg/httpclient"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getMSGraphClient(t *testing.T) (*msgraphsdk.GraphServiceClient, *azidentity.DefaultAzureCredential) {
	httpClient := &http.Client{}
	msGraphClient, azureCredential, err := auth.NewMSGraphClient(httpClient)
	require.NoError(t, err)
	return msGraphClient, azureCredential
}

func TestCollector_scrapeDevices(t *testing.T) {

	var (
		ok       bool
		tenantID string
	)

	if tenantID, ok = os.LookupEnv("AZURE_TENANT_ID"); !ok {
		t.Skip("no AZURE_TENANT_ID environment variable set")
	}

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	msGraphClient, azureCredential := getMSGraphClient(t)

	httpClient := httpclient.New(prometheus.NewRegistry())
	httpClient.WithAzureCredential(azureCredential)

	collector := NewCollector(logger, tenantID, msGraphClient, httpClient.GetHTTPClient())

	// TODO: Go 1.24: Change to t.Context()
	metrics, err := collector.scrapeDevices(context.Background())
	require.NoError(t, err)

	assert.NotEmpty(t, metrics)

	allMetrics, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)

	assert.NotEmpty(t, allMetrics)
	assert.Contains(t, allMetrics, "m365_intune_device_count")
}

func TestCollector_scrapeVppTokens(t *testing.T) {

	var (
		ok       bool
		tenantID string
	)

	if tenantID, ok = os.LookupEnv("AZURE_TENANT_ID"); !ok {
		t.Skip("no AZURE_TENANT_ID environment variable set")
	}

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	msGraphClient, azureCredential := getMSGraphClient(t)

	httpClient := httpclient.New(prometheus.NewRegistry())
	httpClient.WithAzureCredential(azureCredential)

	collector := NewCollector(logger, tenantID, msGraphClient, httpClient.GetHTTPClient())

	// TODO: Go 1.24: Change to t.Context()
	metrics, err := collector.scrapeVppTokens(context.Background())
	require.NoError(t, err)

	// VPP tokens might not exist in all tenants, so we just check that the function doesn't error
	// and returns metrics (even if empty)
	allMetrics, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)

	// If VPP tokens exist, we should see the metrics
	if len(metrics) > 0 {
		assert.Contains(t, allMetrics, "m365_intune_vpp_status")
		assert.Contains(t, allMetrics, "m365_intune_vpp_expiry")
	}
}

func TestCollector_scrapeDepOnboardingSettings(t *testing.T) {

	var (
		ok       bool
		tenantID string
	)

	if tenantID, ok = os.LookupEnv("AZURE_TENANT_ID"); !ok {
		t.Skip("no AZURE_TENANT_ID environment variable set")
	}

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	msGraphClient, azureCredential := getMSGraphClient(t)

	httpClient := httpclient.New(prometheus.NewRegistry())
	httpClient.WithAzureCredential(azureCredential)

	collector := NewCollector(logger, tenantID, msGraphClient, httpClient.GetHTTPClient())

	// TODO: Go 1.24: Change to t.Context()
	metrics, err := collector.scrapeDepOnboardingSettings(context.Background())
	require.NoError(t, err)

	// DEP onboarding settings might not exist in all tenants, so we just check that the function doesn't error
	// and returns metrics (even if empty)
	allMetrics, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)

	// If DEP onboarding settings exist, we should see the metrics
	if len(metrics) > 0 {
		assert.Contains(t, allMetrics, "m365_intune_dep_token_expiry")
	}
}

func TestCollector_scrapeApplePushNotificationCertificate(t *testing.T) {

	var (
		ok       bool
		tenantID string
	)

	if tenantID, ok = os.LookupEnv("AZURE_TENANT_ID"); !ok {
		t.Skip("no AZURE_TENANT_ID environment variable set")
	}

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	msGraphClient, azureCredential := getMSGraphClient(t)

	httpClient := httpclient.New(prometheus.NewRegistry())
	httpClient.WithAzureCredential(azureCredential)

	collector := NewCollector(logger, tenantID, msGraphClient, httpClient.GetHTTPClient())

	// TODO: Go 1.24: Change to t.Context()
	metrics, err := collector.scrapeApplePushNotificationCertificate(context.Background())
	require.NoError(t, err)

	// Apple Push Notification Certificate might not exist in all tenants, so we just check that the function doesn't error
	// and returns metrics (even if empty)
	allMetrics, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)

	// If Apple Push Notification Certificate exists, we should see the metrics
	if len(metrics) > 0 {
		assert.Contains(t, allMetrics, "m365_intune_apn_expiry")
	}
}
