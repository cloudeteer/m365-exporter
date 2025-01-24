package servicehealth

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/cloudeteer/m365-exporter/pkg/conf"
	"github.com/cloudeteer/m365-exporter/pkg/util"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
)

const subsystem = "servicehealth"

const (
	serviceOperationalStatus = iota
	investigatingStatus
	restoringServiceStatus
	verifyingServiceStatus
	serviceRestoredStatus
	postIncidentReviewPublishedStatus
	serviceDegradationStatus
	serviceInterruptionStatus
	extendedRecoveryStatus
	falsePositiveStatus
	investigationSuspendedStatus
)

var status = map[int]string{
	serviceOperationalStatus:          "serviceOperational",
	investigatingStatus:               "investigating",
	restoringServiceStatus:            "restoringService",
	verifyingServiceStatus:            "verifyingService",
	serviceRestoredStatus:             "serviceRestored",
	postIncidentReviewPublishedStatus: "postIncidentReviewPublished",
	serviceDegradationStatus:          "serviceDegradation",
	serviceInterruptionStatus:         "serviceInterruption",
	extendedRecoveryStatus:            "extendedRecovery",
	falsePositiveStatus:               "falsePositive",
	investigationSuspendedStatus:      "investigationSuspended",
}

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	infoDesc   *prometheus.Desc
	healthDesc *prometheus.Desc
	issueDesc  *prometheus.Desc
}

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),
		healthDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, "service", "health"),
			"represents the health status of a service. For the status mapping see the m365_service_health_info metric.",
			[]string{"service_name", "service_id"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		infoDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, "service", "health_info"),
			"companion metric for the service health metric. It is used to map the status to a number.",
			[]string{"use"},
			prometheus.Labels{
				"bad":                         "-1",
				"serviceOperational":          "0",
				"investigating":               "1",
				"restoringService":            "2",
				"verifyingService":            "3",
				"serviceRestored":             "4",
				"postIncidentReviewPublished": "5",
				"serviceDegradation":          "6",
				"serviceInterruption":         "7",
				"extendedRecovery":            "8",
				"falsePositive":               "9",
				"investigationSuspended":      "10",
			},
		),
		issueDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, "service", "health_issue"),
			"health issue of a specific service",
			[]string{"service_name", "classification", "issue_create_timestamp", "title", "issue_id", "issue_close_timestamp"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
	}
}

func (c *Collector) StartBackgroundWorker(ctx context.Context, interval time.Duration) {
	go c.ScrapeWorker(ctx, c.logger, interval, c.ScrapeMetrics)
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.BaseCollector.Describe(ch)

	ch <- c.infoDesc
	ch <- c.healthDesc
	ch <- c.issueDesc
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	// set a timeout for the request
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	metrics := make([]prometheus.Metric, 0)
	metrics = append(metrics, prometheus.MustNewConstMetric(
		c.infoDesc,
		prometheus.GaugeValue,
		1,
		"info",
	))

	healthOverviewMetrics, err := c.getHealthOverviewMetrics(ctx)
	if err != nil {
		return nil, err
	}

	healthIssueMetrics, err := c.getHealthIssueMetrics(ctx)
	if err != nil {
		return nil, err
	}

	metrics = slices.Concat(metrics, healthOverviewMetrics, healthIssueMetrics)

	return metrics, nil
}

func (c *Collector) getHealthOverviewMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	results, err := c.GraphClient().Admin().ServiceAnnouncement().HealthOverviews().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get service health overviews: %w", util.GetOdataError(err))
	}

	metrics := make([]prometheus.Metric, 0, len(results.GetValue()))

	for _, result := range results.GetValue() {
		var foundOrphans bool

		for keyStatus, valueStatus := range status {
			if valueStatus != result.GetStatus().String() {
				continue
			}

			metrics = append(metrics, prometheus.MustNewConstMetric(
				c.healthDesc,
				prometheus.GaugeValue,
				float64(keyStatus),
				*result.GetService(), *result.GetId(),
			))

			foundOrphans = true

			break
		}

		if foundOrphans {
			continue
		}

		// If the status cannot be mapped to a int it get´s -1. This happens e.g. if microsoft decides to create a new status.
		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.healthDesc,
			prometheus.GaugeValue,
			float64(-1),
			*result.GetService(), *result.GetId(),
		))

		c.logger.ErrorContext(ctx, fmt.Sprintf("Service %s has a status that is not tracked. Status is: %s", *result.GetId(), result.GetStatus()))
	}

	return metrics, nil
}

func (c *Collector) getHealthIssueMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	result, err := c.GraphClient().Admin().ServiceAnnouncement().Issues().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get service issues: %w", util.GetOdataError(err))
	}

	pageIterator, err := msgraphcore.NewPageIterator[*models.ServiceHealthIssue](
		result,
		c.GraphClient().GetAdapter(),
		models.CreateServiceHealthIssueCollectionResponseFromDiscriminatorValue,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to page through service issues: %w", util.GetOdataError(err))
	}

	metrics := make([]prometheus.Metric, 0, 50)

	if err = pageIterator.Iterate(ctx, func(issue *models.ServiceHealthIssue) bool {
		if !*issue.GetIsResolved() {
			metrics = append(metrics, prometheus.MustNewConstMetric(
				c.issueDesc,
				prometheus.GaugeValue,
				float64(1),
				*issue.GetService(),
				issue.GetClassification().String(),
				strconv.FormatInt(issue.GetStartDateTime().Unix(), 10),
				*issue.GetTitle(),
				*issue.GetId(),
				"0",
			))
		} else {
			var issueCloseTimestamp int64
			if issue.GetEndDateTime() != nil {
				issueCloseTimestamp = issue.GetEndDateTime().Unix()
			}

			threshold := viper.GetInt(conf.KeyserviceHealthIssueKeepDays) * 86400
			current := time.Now().Unix()

			if (current - int64(threshold)) < issueCloseTimestamp {
				metrics = append(metrics, prometheus.MustNewConstMetric(
					c.issueDesc,
					prometheus.GaugeValue,
					float64(0),
					*issue.GetService(),
					issue.GetClassification().String(),
					strconv.FormatInt(issue.GetStartDateTime().Unix(), 10),
					*issue.GetTitle(),
					*issue.GetId(),
					strconv.FormatInt(issue.GetEndDateTime().Unix(), 10),
				))
			}
		}

		// Return true to continue the iteration
		return true
	}); err != nil {
		return nil, fmt.Errorf("failed to iterate through service issues: %w", err)
	}

	return metrics, nil
}
