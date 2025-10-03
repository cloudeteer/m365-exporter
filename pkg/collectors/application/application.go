package application

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/applications"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/prometheus/client_golang/prometheus"
)

const subsystem = "application"

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	secretExpirationDesc *prometheus.Desc
	secretExpiredDesc    *prometheus.Desc
}

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),

		secretExpirationDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "client_secret_expiration_timestamp"),
			"The expiration timestamp of the client secret (Unix timestamp)",
			[]string{"appName", "appID", "secretName", "keyID"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		secretExpiredDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "client_secret_expired"),
			"Whether the client secret has expired (1 = expired, 0 = valid)",
			[]string{"appName", "appID", "secretName", "keyID"},
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

	ch <- c.secretExpirationDesc
	ch <- c.secretExpiredDesc
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	query := &applications.ApplicationsRequestBuilderGetQueryParameters{
		Select: []string{"id", "appId", "displayName", "passwordCredentials"},
	}
	request := &applications.ApplicationsRequestBuilderGetRequestConfiguration{
		QueryParameters: query,
	}

	appsRequest, err := c.GraphClient().Applications().Get(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch application: %w", err)
	}

	appIterator, err := graphcore.NewPageIterator[*models.Application](
		appsRequest,
		c.GraphClient().GetAdapter(),
		models.CreateApplicationCollectionResponseFromDiscriminatorValue,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create page iterator: %w", err)
	}

	metrics, err := c.iterateThroughApplications(ctx, appIterator)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

func (c *Collector) iterateThroughApplications(ctx context.Context, iterator *graphcore.PageIterator[*models.Application]) ([]prometheus.Metric, error) {
	metrics := make([]prometheus.Metric, 0, 100)
	now := time.Now()

	err := iterator.Iterate(
		ctx,
		func(app *models.Application) bool {
			appName := *app.GetDisplayName()
			appID := *app.GetAppId()

			passwordCreds := app.GetPasswordCredentials()
			if passwordCreds == nil || len(passwordCreds) == 0 {
				return true
			}

			for _, cred := range passwordCreds {
				if cred.GetEndDateTime() == nil {
					c.logger.Warn("password credential has no end date",
						slog.String("appName", appName),
						slog.String("appID", appID))
					continue
				}

				endDate := *cred.GetEndDateTime()
				keyID := ""
				if cred.GetKeyId() != nil {
					keyID = cred.GetKeyId().String()
				}

				secretName := ""
				if cred.GetDisplayName() != nil {
					secretName = *cred.GetDisplayName()
				} else {
					secretName = keyID[:8] // Use first 8 chars of keyID as fallback
				}

				expirationTimestamp := float64(endDate.Unix())
				isExpired := float64(0)
				if now.After(endDate) {
					isExpired = float64(1)
				}

				metrics = append(metrics, prometheus.MustNewConstMetric(
					c.secretExpirationDesc,
					prometheus.GaugeValue,
					expirationTimestamp,
					appName,
					appID,
					secretName,
					keyID,
				))

				metrics = append(metrics, prometheus.MustNewConstMetric(
					c.secretExpiredDesc,
					prometheus.GaugeValue,
					isExpired,
					appName,
					appID,
					secretName,
					keyID,
				))
			}

			return true
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to iterate through application: %w", err)
	}

	return metrics, nil
}
