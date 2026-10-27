package hunting_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudeteer/m365-exporter/internal/testutil"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/hunting"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrapePatchQuery(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"schema": [
				{"name": "DeviceId", "type": "String"},
				{"name": "OSPlatform", "type": "String"},
				{"name": "AssessmentStatus", "type": "String"},
				{"name": "PatchStatus", "type": "String"},
				{"name": "SecurityCount", "type": "Int64"},
				{"name": "CriticalCount", "type": "Int64"}
			],
			"results": [{
				"DeviceId": "dev-1",
				"OSPlatform": "linux",
				"AssessmentStatus": "Succeeded",
				"PatchStatus": "Ready",
				"SecurityCount": 4,
				"CriticalCount": 2
			}]
		}`)
	}))
	t.Cleanup(srv.Close)

	collector := newTestCollector(t, srv.URL, patchSettings())
	metrics, err := collector.ScrapeMetrics(context.Background())
	require.NoError(t, err)

	text, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)
	assert.Contains(t, text, `m365_hunting_device_patch_status{device_id="dev-1",kind="assessment",os_platform="linux",tenant="contoso"} 1`)
	assert.Contains(t, text, `m365_hunting_device_patch_status{device_id="dev-1",kind="patch",os_platform="linux",tenant="contoso"} 1`)
	assert.Contains(t, text, `m365_hunting_device_patch_count{classification="security",device_id="dev-1",os_platform="linux",tenant="contoso"} 4`)
	assert.Contains(t, text, `m365_hunting_device_patch_count{classification="critical",device_id="dev-1",os_platform="linux",tenant="contoso"} 2`)
	assert.NotContains(t, text, "m365_hunting_device_patch{")
	assert.Contains(t, text, `m365_hunting_query_success{query="m365_hunting_device_patch"} 1`)
}

func TestScrapeKeepsSeriesWhenQueryFails(t *testing.T) {
	t.Parallel()

	var phase atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		switch {
		case strings.Contains(string(body), "query-a"):
			if phase.Load() == 1 {
				http.Error(w, "bad query", http.StatusBadRequest)

				return
			}

			writeCount(w, 5)
		case strings.Contains(string(body), "query-b"):
			value := 7
			if phase.Load() == 1 {
				value = 9
			}

			writeCount(w, value)
		default:
			http.Error(w, "unknown query", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	collector := newTestCollector(t, srv.URL, hunting.Settings{Queries: []hunting.Query{
		{Metric: "m365_hunting_a", Query: "query-a", Fields: []hunting.Field{{Name: "n", Type: "value"}}},
		{Metric: "m365_hunting_b", Query: "query-b", Fields: []hunting.Field{{Name: "n", Type: "value"}}},
	}})

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	collector.StartBackgroundWorker(ctx, 20*time.Millisecond)

	require.Eventually(t, func() bool {
		return gaugeIs(reg, "m365_hunting_a", nil, 5) && gaugeIs(reg, "m365_hunting_query_success", map[string]string{"query": "m365_hunting_a"}, 1)
	}, 3*time.Second, 10*time.Millisecond)

	phase.Store(1)

	require.Eventually(t, func() bool {
		_, err := reg.Gather()
		if err != nil {
			return false
		}

		return gaugeIs(reg, "m365_hunting_query_success", map[string]string{"query": "m365_hunting_a"}, 0) &&
			gaugeIs(reg, "m365_hunting_a", nil, 5) &&
			gaugeIs(reg, "m365_hunting_b", nil, 9) &&
			gaugeIs(reg, "m365_hunting_query_success", map[string]string{"query": "m365_hunting_b"}, 1)
	}, 3*time.Second, 10*time.Millisecond)
}

func TestScrapeTimeout(t *testing.T) {
	t.Parallel()

	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)

		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()

		select {
		case <-r.Context().Done():
		case <-timer.C:
		}
	}))
	t.Cleanup(func() {
		blocked.CloseClientConnections()
		blocked.Close()
	})

	collector := newTestCollector(t, blocked.URL, hunting.Settings{Queries: []hunting.Query{{
		Metric:  "m365_hunting_slow",
		Query:   "DeviceInfo",
		Timeout: "100ms",
		Fields:  []hunting.Field{{Name: "n", Type: "value"}},
	}}})

	metrics, err := collector.ScrapeMetrics(context.Background())
	require.NoError(t, err)

	text, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)
	assert.Contains(t, text, `m365_hunting_query_success{query="m365_hunting_slow"} 0`)
	assert.NotContains(t, text, "m365_hunting_slow{")
}

func TestScrapeAcceptsAllRows(t *testing.T) {
	t.Parallel()

	const rows = 1001

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		values := make([]int, rows)
		for i := range values {
			values[i] = i + 1
		}

		writeCountRows(w, values...)
	}))
	t.Cleanup(srv.Close)

	collector := newTestCollector(t, srv.URL, hunting.Settings{Queries: []hunting.Query{{
		Metric: "m365_hunting_rows",
		Query:  "DeviceInfo",
		Fields: []hunting.Field{
			{Name: "id", Type: "id"},
			{Name: "n", Type: "value"},
		},
	}}})

	metrics, err := collector.ScrapeMetrics(context.Background())
	require.NoError(t, err)

	text, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)
	assert.Contains(t, text, `m365_hunting_query_success{query="m365_hunting_rows"} 1`)
	assert.Contains(t, text, `m365_hunting_query_results{query="m365_hunting_rows"} 1001`)
	assert.Contains(t, text, `m365_hunting_rows{id="a",tenant="contoso"} 1001`)
}

func TestScrapeMissingAndNullLabelsAreOneSeries(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"results": [
				{"n": 1},
				{"DeviceId": null, "n": 2}
			]
		}`)
	}))
	t.Cleanup(srv.Close)

	collector := newTestCollector(t, srv.URL, hunting.Settings{Queries: []hunting.Query{{
		Metric: "m365_hunting_sparse",
		Query:  "DeviceInfo",
		Fields: []hunting.Field{
			{Name: "DeviceId", Target: "device_id", Type: "id"},
			{Name: "n", Type: "value"},
		},
	}}})

	metrics, err := collector.ScrapeMetrics(context.Background())
	require.NoError(t, err)

	text, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)
	assert.Contains(t, text, `m365_hunting_sparse{device_id="",tenant="contoso"} 2`)
	assert.NotContains(t, text, `m365_hunting_sparse{device_id="",tenant="contoso"} 1`)
}

func TestScrapeAcceptsLargeResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"n":1,"pad":"`)
		_, _ = io.WriteString(w, strings.Repeat("a", (32<<20)+1))
		_, _ = io.WriteString(w, `"}]}`)
	}))
	t.Cleanup(srv.Close)

	collector := newTestCollector(t, srv.URL, hunting.Settings{Queries: []hunting.Query{{
		Metric: "m365_hunting_huge",
		Query:  "DeviceInfo",
		Fields: []hunting.Field{{Name: "n", Type: "value"}},
	}}})

	metrics, err := collector.ScrapeMetrics(context.Background())
	require.NoError(t, err)

	text, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)
	assert.Contains(t, text, `m365_hunting_query_success{query="m365_hunting_huge"} 1`)
	assert.Contains(t, text, `m365_hunting_huge{tenant="contoso"} 1`)
}

func TestScrapeLastRowWins(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeCountRows(w, 1, 3)
	}))
	t.Cleanup(srv.Close)

	collector := newTestCollector(t, srv.URL, hunting.Settings{Queries: []hunting.Query{{
		Metric: "m365_hunting_latest",
		Query:  "DeviceInfo",
		Fields: []hunting.Field{
			{Name: "id", Type: "id"},
			{Name: "n", Type: "value"},
		},
	}}})

	metrics, err := collector.ScrapeMetrics(context.Background())
	require.NoError(t, err)

	text, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)
	assert.Contains(t, text, `m365_hunting_latest{id="a",tenant="contoso"} 3`)
	assert.NotContains(t, text, `m365_hunting_latest{id="a",tenant="contoso"} 1`)
}

func TestScrapeThrottled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	collector := newTestCollector(t, srv.URL, hunting.Settings{Queries: []hunting.Query{{
		Metric: "m365_hunting_throttled",
		Query:  "DeviceInfo",
		Value:  floatPtr(1),
	}}})

	metrics, err := collector.ScrapeMetrics(context.Background())
	require.NoError(t, err)

	text, err := testutil.MetricsToText(t, metrics)
	require.NoError(t, err)
	assert.Contains(t, text, `m365_hunting_query_success{query="m365_hunting_throttled"} 0`)
}

func newTestCollector(t *testing.T, endpoint string, settings hunting.Settings) *hunting.Collector {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	collector, err := hunting.NewCollector(logger, "contoso", http.DefaultClient, settings)
	require.NoError(t, err)

	collector.Endpoint = endpoint

	return collector
}

func patchSettings() hunting.Settings {
	return hunting.Settings{Queries: []hunting.Query{{
		Metric:   "m365_hunting_device_patch",
		Query:    "DeviceInfo",
		Timespan: "P1D",
		Fields: []hunting.Field{
			{Name: "DeviceId", Target: "device_id", Type: "id"},
			{Name: "OSPlatform", Target: "os_platform", Type: "id"},
			{
				Name:   "AssessmentStatus",
				Type:   "value",
				Metric: "m365_hunting_device_patch_status",
				Labels: map[string]string{"kind": "assessment"},
				Filters: []hunting.Filter{
					{Type: "regexp", Regexp: "^Succeeded$", Replacement: "1"},
					{Type: "regexp", Regexp: "^Failed$", Replacement: "3"},
					{Type: "regexp", Regexp: "^[A-Za-z]+$", Replacement: "-1"},
				},
			},
			{
				Name:   "PatchStatus",
				Type:   "value",
				Metric: "m365_hunting_device_patch_status",
				Labels: map[string]string{"kind": "patch"},
				Filters: []hunting.Filter{
					{Type: "regexp", Regexp: "^Ready$", Replacement: "1"},
					{Type: "regexp", Regexp: "^Failed$", Replacement: "3"},
					{Type: "regexp", Regexp: "^[A-Za-z]+$", Replacement: "-1"},
				},
			},
			{
				Name:   "SecurityCount",
				Type:   "value",
				Metric: "m365_hunting_device_patch_count",
				Labels: map[string]string{"classification": "security"},
			},
			{
				Name:   "CriticalCount",
				Type:   "value",
				Metric: "m365_hunting_device_patch_count",
				Labels: map[string]string{"classification": "critical"},
			},
		},
	}}}
}

func writeCount(w http.ResponseWriter, value int) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"schema":[{"name":"n","type":"Int64"}],"results":[{"n":%d}]}`, value)
}

func writeCountRows(w http.ResponseWriter, values ...int) {
	w.Header().Set("Content-Type", "application/json")

	rows := make([]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, fmt.Sprintf(`{"id":"a","n":%d}`, value))
	}

	_, _ = fmt.Fprintf(w, `{"schema":[{"name":"id","type":"String"},{"name":"n","type":"Int64"}],"results":[%s]}`, strings.Join(rows, ","))
}

func gaugeIs(reg *prometheus.Registry, name string, labels map[string]string, want float64) bool {
	families, err := reg.Gather()
	if err != nil {
		return false
	}

	for _, family := range families {
		if family.GetName() != name {
			continue
		}

		for _, metric := range family.GetMetric() {
			if !labelsMatch(metric, labels) {
				continue
			}

			return metric.GetGauge().GetValue() == want
		}
	}

	return false
}

func labelsMatch(metric *dto.Metric, want map[string]string) bool {
	have := map[string]string{}

	for _, label := range metric.GetLabel() {
		have[label.GetName()] = label.GetValue()
	}

	for key, value := range want {
		if have[key] != value {
			return false
		}
	}

	return true
}
