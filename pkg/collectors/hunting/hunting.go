package hunting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	defaultEndpoint = "https://graph.microsoft.com/v1.0/security/runHuntingQuery"
)

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type metricDesc struct {
	desc   *prometheus.Desc
	labels []string
}

// Collector runs configured advanced-hunting queries and caches the gauges they produce.
type Collector struct {
	abstract.BaseCollector

	logger   *slog.Logger
	client   *http.Client
	settings Settings

	// Endpoint is the Graph runHuntingQuery URL. Tests replace it with an httptest server.
	Endpoint string

	descs        map[string]metricDesc
	successDesc  *prometheus.Desc
	resultsDesc  *prometheus.Desc
	durationDesc *prometheus.Desc
	lastDesc     *prometheus.Desc

	series      map[string][]prometheus.Metric
	lastSuccess map[string]time.Time
}

type huntingRequest struct {
	Query       string `json:"Query"`
	Timespan    string `json:"Timespan,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

type huntingResponse struct {
	Schema  []huntingColumn  `json:"schema"`
	Results []map[string]any `json:"results"`
}

type huntingColumn struct {
	Name string `json:"name"`
}

// NewCollector validates settings and builds one descriptor per metric name.
func NewCollector(logger *slog.Logger, tenant string, httpClient *http.Client, settings Settings) (*Collector, error) {
	prepared, err := Prepare(settings)
	if err != nil {
		return nil, err
	}

	collector := &Collector{
		BaseCollector: abstract.NewBaseCollector(nil, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),
		client:        httpClient,
		settings:      prepared,
		Endpoint:      defaultEndpoint,
		descs:         map[string]metricDesc{},
		series:        map[string][]prometheus.Metric{},
		lastSuccess:   map[string]time.Time{},
		successDesc: prometheus.NewDesc(
			operationalName(nameQuerySuccess),
			"1 if the last run of this hunting query succeeded.",
			[]string{labelQuery},
			nil,
		),
		resultsDesc: prometheus.NewDesc(
			operationalName(nameQueryResults),
			"Number of rows returned by the last successful hunting query. 0 when the query failed.",
			[]string{labelQuery},
			nil,
		),
		durationDesc: prometheus.NewDesc(
			operationalName(nameQueryDuration),
			"Seconds spent running this hunting query, including a failed attempt.",
			[]string{labelQuery},
			nil,
		),
		lastDesc: prometheus.NewDesc(
			operationalName(nameQueryLastSuccess),
			"Unix time of the last successful run of this hunting query. Stays at the previous value when a run fails.",
			[]string{labelQuery},
			nil,
		),
	}

	constLabels := prometheus.Labels{labelTenant: tenant}

	for _, query := range prepared.Queries {
		for _, name := range metricNames(query) {
			if _, ok := collector.descs[name]; ok {
				return nil, fmt.Errorf("metric %q is used by more than one hunting query", name)
			}

			labels := variableLabels(query, name)
			collector.descs[name] = metricDesc{
				desc: prometheus.NewDesc(
					name,
					metricHelp(query, name),
					labels,
					constLabels,
				),
				labels: labels,
			}
		}
	}

	return collector, nil
}

// StartBackgroundWorker polls the configured queries until ctx is cancelled.
func (c *Collector) StartBackgroundWorker(ctx context.Context, interval time.Duration) {
	go c.ScrapeWorker(ctx, c.logger, interval, c.ScrapeMetrics)
}

// Describe sends the base collector gauges, the per-query gauges, and every configured metric.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.BaseCollector.Describe(ch)

	ch <- c.successDesc

	ch <- c.resultsDesc

	ch <- c.durationDesc

	ch <- c.lastDesc

	for _, spec := range c.descs {
		ch <- spec.desc
	}
}

// ScrapeMetrics runs every configured query.
// It returns a nil error after the loop so one failed query does not freeze the others.
// Per-query success is m365_hunting_query_success. m365_collector_scrape_success means this loop finished.
func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	metrics := make([]prometheus.Metric, 0, len(c.settings.Queries)*4)

	for _, query := range c.settings.Queries {
		metrics = append(metrics, c.scrapeQuery(ctx, query)...)
	}

	return metrics, nil
}

func (c *Collector) scrapeQuery(ctx context.Context, query Query) []prometheus.Metric {
	start := time.Now()

	var (
		panicked any
		stack    string
	)

	metrics := c.runQuery(ctx, query, start, &panicked, &stack)
	if panicked == nil {
		return metrics
	}

	c.logger.ErrorContext(ctx, "panic while scraping hunting query",
		slog.String(labelQuery, query.Metric),
		slog.Any("err", panicked),
		slog.String("stack", stack),
	)

	return c.queryMetrics(ctx, query, start, 0, false)
}

func (c *Collector) runQuery(ctx context.Context, query Query, start time.Time, panicked *any, stack *string) []prometheus.Metric {
	defer func() {
		recovered := recover()
		if recovered != nil {
			*panicked = recovered
			*stack = string(debug.Stack())
		}
	}()

	series, rows, err := c.executeQuery(ctx, query)
	if err != nil {
		c.logger.ErrorContext(ctx, "hunting query failed",
			slog.String(labelQuery, query.Metric),
			slog.Any("err", err),
		)

		return c.queryMetrics(ctx, query, start, 0, false)
	}

	c.series[query.Metric] = series
	c.lastSuccess[query.Metric] = time.Now()

	return c.queryMetrics(ctx, query, start, rows, true)
}

func (c *Collector) queryMetrics(ctx context.Context, query Query, start time.Time, rows int, ok bool) []prometheus.Metric {
	metrics := append([]prometheus.Metric{}, c.series[query.Metric]...)

	successValue := 0.0
	if ok {
		successValue = 1
	}

	lastValue := 0.0
	if last, exists := c.lastSuccess[query.Metric]; exists {
		lastValue = float64(last.Unix())
	}

	gauges := []struct {
		desc  *prometheus.Desc
		value float64
	}{
		{desc: c.successDesc, value: successValue},
		{desc: c.resultsDesc, value: float64(rows)},
		{desc: c.durationDesc, value: time.Since(start).Seconds()},
		{desc: c.lastDesc, value: lastValue},
	}

	for _, gauge := range gauges {
		metric, err := prometheus.NewConstMetric(gauge.desc, prometheus.GaugeValue, gauge.value, query.Metric)
		if err != nil {
			c.logger.ErrorContext(ctx, "failed to build hunting query gauge",
				slog.String(labelQuery, query.Metric),
				slog.Any("err", err),
			)

			continue
		}

		metrics = append(metrics, metric)
	}

	return metrics
}

func (c *Collector) executeQuery(ctx context.Context, query Query) ([]prometheus.Metric, int, error) {
	body, status, err := c.postQuery(ctx, query)
	if err != nil {
		return nil, 0, err
	}

	if status == http.StatusTooManyRequests {
		return nil, 0, fmt.Errorf("hunting query throttled (retry-after %q): %s", headerRetryAfter(body), truncate(body.payload))
	}

	if status != http.StatusOK {
		return nil, 0, fmt.Errorf("hunting query status %d: %s", status, truncate(body.payload))
	}

	parsed, err := decodeHunting(body.payload)
	if err != nil {
		return nil, 0, err
	}

	c.warnMissingFields(ctx, query, parsed.Schema)

	metrics, dropped, err := c.metricsFromRows(query, parsed.Results)
	if err != nil {
		return nil, 0, err
	}

	if dropped > 0 {
		c.logger.WarnContext(ctx, "dropped hunting samples",
			slog.String(labelQuery, query.Metric),
			slog.Int("dropped", dropped),
		)
	}

	return metrics, len(parsed.Results), nil
}

type queryResponse struct {
	payload []byte
	header  http.Header
}

func (c *Collector) postQuery(ctx context.Context, query Query) (queryResponse, int, error) {
	queryCtx, cancel := context.WithTimeout(ctx, query.TimeoutResolved())
	defer cancel()

	payload, err := json.Marshal(huntingRequest{
		Query:       query.Query,
		Timespan:    query.Timespan,
		WorkspaceID: query.WorkspaceID,
	})
	if err != nil {
		return queryResponse{}, 0, fmt.Errorf("encode hunting query: %w", err)
	}

	req, err := http.NewRequestWithContext(queryCtx, http.MethodPost, c.endpointURL(), bytes.NewReader(payload))
	if err != nil {
		return queryResponse{}, 0, fmt.Errorf("create hunting request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.client.Do(req)
	if err != nil {
		return queryResponse{}, 0, fmt.Errorf("run hunting query: %w", err)
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			c.logger.ErrorContext(ctx, "error closing hunting response body",
				slog.String(labelQuery, query.Metric),
				slog.Any("err", err),
			)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return queryResponse{}, resp.StatusCode, fmt.Errorf("read hunting response: %w", err)
	}

	return queryResponse{payload: respBody, header: resp.Header}, resp.StatusCode, nil
}

func headerRetryAfter(resp queryResponse) string {
	if resp.header == nil {
		return ""
	}

	return resp.header.Get("Retry-After")
}

func decodeHunting(payload []byte) (huntingResponse, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	var parsed huntingResponse

	err := decoder.Decode(&parsed)
	if err != nil {
		return huntingResponse{}, fmt.Errorf("decode hunting query: %w", err)
	}

	return parsed, nil
}

func (c *Collector) warnMissingFields(ctx context.Context, query Query, schema []huntingColumn) {
	if len(schema) == 0 {
		return
	}

	present := map[string]struct{}{}

	for _, column := range schema {
		present[column.Name] = struct{}{}
	}

	for _, field := range query.Fields {
		if _, ok := present[field.Name]; ok {
			continue
		}

		c.logger.WarnContext(ctx, "hunting field is not in the result schema",
			slog.String(labelQuery, query.Metric),
			slog.String("field", field.Name),
		)
	}
}

func (c *Collector) metricsFromRows(query Query, rows []map[string]any) ([]prometheus.Metric, int, error) {
	order := make([]string, 0)
	samples := map[string]Sample{}
	dropped := 0

	for _, row := range rows {
		rowSamples, rowDropped := MapRow(query, row)
		dropped += rowDropped

		for _, sample := range rowSamples {
			key, err := c.seriesKey(sample)
			if err != nil {
				return nil, dropped, err
			}

			if _, ok := samples[key]; !ok {
				order = append(order, key)
			}

			samples[key] = sample
		}
	}

	metrics := make([]prometheus.Metric, 0, len(order))

	for _, key := range order {
		metric, err := c.sampleMetric(samples[key])
		if err != nil {
			return nil, dropped, err
		}

		metrics = append(metrics, metric)
	}

	return metrics, dropped, nil
}

func (c *Collector) seriesKey(sample Sample) (string, error) {
	_, values, err := c.exportLabels(sample)
	if err != nil {
		return "", err
	}

	return sample.Name + "\x00" + strings.Join(values, "\x00"), nil
}

func (c *Collector) sampleMetric(sample Sample) (prometheus.Metric, error) {
	spec, values, err := c.exportLabels(sample)
	if err != nil {
		return nil, err
	}

	metric, err := prometheus.NewConstMetric(spec.desc, prometheus.GaugeValue, sample.Value, values...)
	if err != nil {
		return nil, fmt.Errorf("build metric %q: %w", sample.Name, err)
	}

	return metric, nil
}

// exportLabels returns descriptor label values in the same order NewConstMetric receives them.
// A missing label and an empty label both become "", so they identify one series.
func (c *Collector) exportLabels(sample Sample) (metricDesc, []string, error) {
	spec, ok := c.descs[sample.Name]
	if !ok {
		return metricDesc{}, nil, fmt.Errorf("no descriptor for metric %q", sample.Name)
	}

	values := make([]string, len(spec.labels))

	for i, name := range spec.labels {
		values[i] = sample.Labels[name]
	}

	return spec, values, nil
}

func (c *Collector) endpointURL() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}

	return defaultEndpoint
}

func truncate(payload []byte) string {
	const maxLen = 512

	if len(payload) > maxLen {
		return string(payload[:maxLen])
	}

	return string(payload)
}
