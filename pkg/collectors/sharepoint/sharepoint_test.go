package sharepoint

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/cloudeteer/m365-exporter/pkg/auth"
	"github.com/cloudeteer/m365-exporter/pkg/httpclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestCollector_findSharepoints(t *testing.T) {
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

	collector := NewCollector(logger, tenantID, msGraphClient, httpClient.GetHTTPClient())

	_, err = collector.findSharepoints(context.Background())
	require.NoError(t, err)
}
