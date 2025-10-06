package application_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/cloudeteer/m365-exporter/internal/testutil"
	"github.com/cloudeteer/m365-exporter/pkg/auth"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/application"
	"github.com/cloudeteer/m365-exporter/pkg/httpclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_ScrapeApplicationWithFilterMetrics(t *testing.T) {
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

	collectorWithFilter := application.NewCollector(logger, tenantID, msGraphClient, application.Settings{
		Filter: "signInAudience eq 'AzureADMyOrg'",
	})

	// TODO: Go 1.24: Change to t.Context()
	metricsWIthFilter, err := collectorWithFilter.ScrapeMetrics(context.TODO())
	require.NoError(t, err)

	assert.NotEmpty(t, metricsWIthFilter)

	allMetricsWithFilter, err := testutil.MetricsToText(t, metricsWIthFilter)
	require.NoError(t, err)

	assert.NotEmpty(t, allMetricsWithFilter)
	assert.Contains(t, allMetricsWithFilter, "m365_application_client_secret_expiration_timestamp")
	assert.Contains(t, allMetricsWithFilter, "m365_application_client_secret_expired")
}
