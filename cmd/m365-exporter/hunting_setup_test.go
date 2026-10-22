package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/cloudeteer/m365-exporter/pkg/conf"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestSetupHuntingDisabledIgnoresInvalidQueries(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set(conf.KeyHuntingEnabled, false)
	viper.Set("hunting.queries", []any{
		map[string]any{"metric": "not a metric"},
	})

	err := setupMetricsCollectors(context.Background(), discardLogger(), prometheus.NewRegistry(), "tenant", nil, http.DefaultClient)
	require.NoError(t, err)
}

func TestSetupHuntingRequiresQueries(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set(conf.KeyHuntingEnabled, true)

	err := setupMetricsCollectors(context.Background(), discardLogger(), prometheus.NewRegistry(), "tenant", nil, http.DefaultClient)
	require.ErrorContains(t, err, "at least one query")
}

func TestSetupHuntingRejectsInterval(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set(conf.KeyHuntingEnabled, true)
	viper.Set(conf.KeyHuntingInterval, "nope")
	viper.Set("hunting.queries", []any{
		map[string]any{
			"metric": "m365_hunting_test",
			"query":  "DeviceInfo",
			"value":  1,
		},
	})

	err := setupMetricsCollectors(context.Background(), discardLogger(), prometheus.NewRegistry(), "tenant", nil, http.DefaultClient)
	require.ErrorContains(t, err, "hunting interval")
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
