package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/prometheus/client_golang/prometheus"
)

const subsystem = "exchange"

const (
	exchangeOnlineAdminAPI = "https://outlook.office365.com/adminapi/beta/%s/InvokeCommand"
)

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	mailflowMessageCount *prometheus.Desc

	httpExchangeAdminBaseURL string
	httpClient               *http.Client
}

func NewCollector(logger *slog.Logger, tenant string, httpClient *http.Client) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(nil, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),
		mailflowMessageCount: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "mailflow_messages"),
			"Number of messages in the mail flow",
			[]string{
				"organization",
				"direction",
				"event_type",
			},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		httpExchangeAdminBaseURL: fmt.Sprintf(exchangeOnlineAdminAPI, tenant),
		httpClient:               httpClient,
	}
}

func (c *Collector) StartBackgroundWorker(ctx context.Context, interval time.Duration) {
	go c.ScrapeWorker(ctx, c.logger, interval, c.ScrapeMetrics)
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.BaseCollector.Describe(ch)

	ch <- c.mailflowMessageCount
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	// set a timeout for the request
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	metrics := make([]prometheus.Metric, 0)
	errs := make([]error, 0)

	mailflowMetrics, err := c.scrapeMailflowMetrics(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("error scraping mailflow metrics: %w", err))
	}

	metrics = append(metrics, mailflowMetrics...)

	return metrics, errors.Join(errs...)
}

//nolint:cyclop
func (c *Collector) scrapeMailflowMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.httpExchangeAdminBaseURL,
		strings.NewReader(`{"CmdletInput": {"CmdletName": "Get-MailFlowStatusReport"}}`),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			c.logger.ErrorContext(ctx, "error closing response body", slog.Any("err", err))
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error unmarshalling response (status %d): %s", resp.StatusCode, body)
	}

	var mailFlowResponse MailFlowResponse

	err = json.Unmarshal(body, &mailFlowResponse)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling response: body %s, error %w", body, err)
	}

	// Find the most recent date in the response
	var lastDateValue time.Time

	for _, mailFlow := range mailFlowResponse.Value {
		date, err := time.Parse("2006-01-02T15:04:05.0000000", mailFlow.Date)
		if err != nil {
			return nil, fmt.Errorf("error parsing date: %w", err)
		}

		// Skip if the date is not after the last date
		// Also, skip "today", because the data is incomplete.
		if !date.After(lastDateValue) || date.After(time.Now().Add(-24*time.Hour)) {
			continue
		}

		// Update the last date
		lastDateValue = date
	}

	metrics := make([]prometheus.Metric, 0)

	// Find the most recent mail flow status
	for _, mailFlow := range mailFlowResponse.Value {
		date, err := time.Parse("2006-01-02T15:04:05.0000000", mailFlow.Date)
		if err != nil {
			return nil, fmt.Errorf("error parsing date: %w", err)
		}

		if !date.Equal(lastDateValue) {
			continue
		}

		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.mailflowMessageCount,
			prometheus.GaugeValue,
			float64(mailFlow.MessageCount),
			mailFlow.Organization,
			mailFlow.Direction,
			mailFlow.EventType,
		))
	}

	return metrics, nil
}
