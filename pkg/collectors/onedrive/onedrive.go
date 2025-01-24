package onedrive

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/cloudeteer/m365-exporter/pkg/util"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/sites"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/prometheus/client_golang/prometheus"
)

const subsystem = "onedrive"

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	totalDesc  *prometheus.Desc
	usedOpts   *prometheus.Desc
	deletedOps *prometheus.Desc

	settings Settings
}

type Settings struct {
	ScrambleSalt  string
	ScrambleNames bool
}

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient, settings Settings) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),

		totalDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "total_available_bytes"),
			"the total amount of available bytes for this onedrive",
			[]string{"owner", "driveType", "driveID"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		usedOpts: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "used_bytes"),
			"number of bytes used on this onedrive",
			[]string{"owner", "driveType", "driveID"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		deletedOps: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "deleted_bytes"),
			"number of bytes in recycle bin",
			[]string{"owner", "driveType", "driveID"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		settings: settings,
	}
}

func (c *Collector) StartBackgroundWorker(ctx context.Context, interval time.Duration) {
	go c.ScrapeWorker(ctx, c.logger, interval, c.ScrapeMetrics)
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.BaseCollector.Describe(ch)

	ch <- c.totalDesc
	ch <- c.usedOpts
	ch <- c.deletedOps
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	errs := make([]error, 0, 2)

	metricsSites, err := c.scrapeMetricsSites(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to scrape sites: %w", err))
	}

	metricsUsers, err := c.scrapeMetricsUsers(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to scrape users: %w", err))
	}

	return slices.Concat(metricsSites, metricsUsers), errors.Join(errs...)
}

func (c *Collector) scrapeMetricsSites(ctx context.Context) ([]prometheus.Metric, error) {
	sharepointSites, err := c.GraphClient().Sites().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sites: %w", util.GetOdataError(err))
	}

	sIterator, err := graphcore.NewPageIterator[*models.Site](
		sharepointSites,
		c.GraphClient().GetAdapter(),
		models.CreateSiteCollectionResponseFromDiscriminatorValue,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create site iterator: %w", util.GetOdataError(err))
	}

	// iterate over sites
	metrics, err := c.iterateThroughSites(ctx, sIterator)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

func (c *Collector) scrapeMetricsUsers(ctx context.Context) ([]prometheus.Metric, error) {
	query := users.UsersRequestBuilderGetQueryParameters{
		Select: []string{"id", "userPrincipalName"},
	}

	config := users.UsersRequestBuilderGetRequestConfiguration{
		QueryParameters: &query,
	}

	user, err := c.GraphClient().Users().Get(ctx, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch users: %w", util.GetOdataError(err))
	}

	uIterator, err := graphcore.NewPageIterator[*models.User](
		user,
		c.GraphClient().GetAdapter(),
		models.CreateUserCollectionResponseFromDiscriminatorValue,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create site iterator: %w", util.GetOdataError(err))
	}

	metrics, err := c.iterateThroughUsers(ctx, uIterator)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

func (c *Collector) iterateThroughSites(ctx context.Context, sIterator *graphcore.PageIterator[*models.Site]) ([]prometheus.Metric, error) {
	metrics := make([]prometheus.Metric, 0, 100)

	err := sIterator.Iterate(ctx, func(site *models.Site) bool {
		// get user onedrive
		query := sites.ItemDriveRequestBuilderGetQueryParameters{
			Select: []string{"quota", "driveType"},
		}

		config := sites.ItemDriveRequestBuilderGetRequestConfiguration{
			QueryParameters: &query,
		}

		// we cannot use the error currently b/c an error is returned if the user does not have a drive, it is not possible to filter for sites
		// which have a drive
		result, err := c.GraphClient().Sites().BySiteId(*site.GetId()).Drive().Get(ctx, &config)
		if err != nil {
			c.logger.DebugContext(ctx, "Encountered an error while iterating through sites: %w", slog.Any("err", util.GetOdataError(err)))

			return true
		}
		// sometimes this is needed, dont ask me why
		if site.GetDisplayName() == nil {
			return true
		}

		// filter for sharepoint document libraries only
		dType := *result.GetDriveType()
		if dType == "documentLibrary" {
			owner := *site.GetDisplayName()
			metrics = append(metrics, prometheus.MustNewConstMetric(
				c.totalDesc,
				prometheus.GaugeValue,
				float64(*result.GetQuota().GetTotal()),
				owner, dType, *site.GetId(),
			))

			metrics = append(metrics, prometheus.MustNewConstMetric(
				c.deletedOps,
				prometheus.GaugeValue,
				float64(*result.GetQuota().GetDeleted()),
				owner, dType, *site.GetId(),
			))

			metrics = append(metrics, prometheus.MustNewConstMetric(
				c.usedOpts,
				prometheus.GaugeValue,
				float64(*result.GetQuota().GetUsed()),
				owner, dType, *site.GetId(),
			))
		}

		return true
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate through sites: %w", util.GetOdataError(err))
	}

	return metrics, nil
}

func (c *Collector) iterateThroughUsers(ctx context.Context, uIterator *graphcore.PageIterator[*models.User]) ([]prometheus.Metric, error) {
	metrics := make([]prometheus.Metric, 0, 100)

	err := uIterator.Iterate(ctx, func(user *models.User) bool {
		// get user onedrive
		query := users.ItemDriveRequestBuilderGetQueryParameters{
			Select: []string{"quota", "driveType"},
		}

		config := users.ItemDriveRequestBuilderGetRequestConfiguration{
			QueryParameters: &query,
		}

		// we cannot use the error currently b/c an error is returned if the user does not have a drive, it is not possible to filter for users
		// which have a drive
		result, err := c.GraphClient().Users().ByUserId(*user.GetId()).Drive().Get(ctx, &config)
		if err != nil {
			c.logger.DebugContext(ctx, "Got an error when getting user drives", slog.Any("err", err))

			return true
		}

		// errors are so unusable here, that I debug them only
		if result == nil {
			c.logger.DebugContext(ctx, "Skipping nil result from getting drives")

			return true
		}

		owner := *user.GetUserPrincipalName()

		// scramble username
		if c.settings.ScrambleNames {
			owner = owner + "+" + c.settings.ScrambleSalt
			owner = fmt.Sprintf("%x", sha256.Sum256([]byte(owner)))
		}

		dType := *result.GetDriveType()
		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.totalDesc,
			prometheus.GaugeValue,
			float64(*result.GetQuota().GetTotal()),
			owner, dType, "",
		))
		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.usedOpts,
			prometheus.GaugeValue,
			float64(*result.GetQuota().GetUsed()),
			owner, dType, "",
		))
		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.deletedOps,
			prometheus.GaugeValue,
			float64(*result.GetQuota().GetDeleted()),
			owner, dType, "",
		))

		return true
	},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to iterate through users: %w", util.GetOdataError(err))
	}

	return metrics, nil
}
