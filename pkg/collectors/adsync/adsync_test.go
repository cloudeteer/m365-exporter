package adsync_test

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
	"github.com/cloudeteer/m365-exporter/pkg/collectors/adsync"
	"github.com/cloudeteer/m365-exporter/pkg/httpclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_ScrapeMetrics(t *testing.T) {
	t.Parallel()
	t.Skip()

	var (
		ok       bool
		tenantID string
	)

	if tenantID, ok = os.LookupEnv("AZURE_TENANT_ID"); !ok {
		t.Skipf("no AZURE_TENANT_ID environment variable set")
	}

	// TODO: Go 1.24: Change to slog.NewDiscardHandler
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// TODO: make this a singleton for all tests
	msGraphClient, azureCredential, err := auth.NewMSGraphClient(http.DefaultClient)
	require.NoError(t, err)

	httpClient := httpclient.New(prometheus.NewRegistry())
	httpClient.WithAzureCredential(azureCredential)

	collector := adsync.NewCollector(logger, tenantID, msGraphClient, httpClient.GetHTTPClient())

	// TODO: Go 1.24: Change to t.Context()
	metrics, err := collector.ScrapeMetrics(context.TODO())
	require.NoError(t, err)

	assert.NotEmpty(t, metrics)

	allMetrics, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)

	assert.NotEmpty(t, allMetrics)
	assert.Contains(t, allMetrics, "m365_adsync_on_premises_last_sync_date_time")
	assert.Contains(t, allMetrics, "m365_adsync_on_premises_sync_enabled")
	assert.Contains(t, allMetrics, "m365_adsync_on_premises_sync_error")

	for _, errorBucket := range []string{
		"All",
		"DataMismatch",
		"DataValidationError",
		"DuplicateAttributeError",
		"FederatedDomainChange",
		"LargeAttribute",
		"Others",
		"RoleMembershipSoftMatchFailure",
	} {
		assert.Regexp(t, fmt.Sprintf(`m365_adsync_on_premises_sync_error{.*error_bucket="%s".*} [0-9.e+-]`, errorBucket), allMetrics)
	}
}
