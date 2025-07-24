package teams

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/teams"
	"github.com/prometheus/client_golang/prometheus"
)

const subsystem = "teams"

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	memberDesc *prometheus.Desc
	ownerDesc  *prometheus.Desc
}

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),

		memberDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "team_member_count"),
			"The number of members in the team",
			[]string{"teamName", "teamID"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		ownerDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "team_owner_count"),
			"the number of owners in the team",
			[]string{"teamName", "teamID"},
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

	ch <- c.memberDesc

	ch <- c.ownerDesc
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	query := &teams.TeamsRequestBuilderGetQueryParameters{
		Select: []string{"id"},
	}
	request := &teams.TeamsRequestBuilderGetRequestConfiguration{
		QueryParameters: query,
	}

	teamsRequest, err := c.GraphClient().Teams().Get(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch teams: %w", err)
	}

	tIterator, err := graphcore.NewPageIterator[*models.Team](
		teamsRequest,
		c.GraphClient().GetAdapter(),
		models.CreateTeamCollectionResponseFromDiscriminatorValue,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page iterator: %w", err)
	}

	metrics, err := c.iterateThroughTeams(ctx, tIterator)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

func (c *Collector) iterateThroughTeams(ctx context.Context, iterator *graphcore.PageIterator[*models.Team]) ([]prometheus.Metric, error) {
	metrics := make([]prometheus.Metric, 0, 100)

	err := iterator.Iterate(
		ctx,
		func(t *models.Team) bool {
			r := c.GraphClient().Teams().ByTeamId(*t.GetId())

			team, err := r.Get(ctx, nil)
			if err != nil {
				return false
			}

			teamName := *team.GetDisplayName()

			metrics = append(metrics, prometheus.MustNewConstMetric(
				c.memberDesc,
				prometheus.GaugeValue,
				float64(*team.GetSummary().GetMembersCount()),
				teamName,
				*team.GetId(),
			))

			metrics = append(metrics, prometheus.MustNewConstMetric(
				c.ownerDesc,
				prometheus.GaugeValue,
				float64(*team.GetSummary().GetOwnersCount()),
				teamName,
				*team.GetId(),
			))

			return true
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to iterate through teams: %w", err)
	}

	return metrics, nil
}
