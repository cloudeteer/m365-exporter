package sharepoint_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/cloudeteer/m365-exporter/internal/testutil"
	"github.com/cloudeteer/m365-exporter/pkg/auth"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/sharepoint"
	"github.com/cloudeteer/m365-exporter/pkg/httpclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_ScrapeMetrics(t *testing.T) {
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
	_, azureCredential, err := auth.NewMSGraphClient(http.DefaultClient)
	require.NoError(t, err)

	// TODO: make this a singleton for all tests
	msGraphClient, azureCredential, err := auth.NewMSGraphClient(http.DefaultClient)

	httpClient := httpclient.New(prometheus.NewRegistry())
	httpClient.WithAzureCredential(azureCredential)

	collector := sharepoint.NewCollector(logger, tenantID, msGraphClient, httpClient.GetHTTPClient())

	// TODO: Go 1.24: Change to t.Context()
	metrics, err := collector.ScrapeMetrics(context.TODO())
	require.NoError(t, err)

	assert.NotEmpty(t, metrics)

	allMetrics, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)

	assert.NotEmpty(t, allMetrics)
	assert.Regexp(t, fmt.Sprintf(`m365_sharepoint_usage_info{.+,tenant="%s",.+} [0-9.e+-]`, tenantID), allMetrics)
}
