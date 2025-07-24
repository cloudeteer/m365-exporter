package license

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/cloudeteer/m365-exporter/pkg/util"
	abstractions "github.com/microsoft/kiota-abstractions-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphgroups "github.com/microsoftgraph/msgraph-sdk-go/groups"
	"github.com/prometheus/client_golang/prometheus"
)

const subsystem = "license"

// Interface guard.
var _ abstract.Collector = (*Collector)(nil)

var capabilityStatuses = map[string]float64{
	"Enabled":   0,
	"Warning":   1,
	"Suspended": 2,
	"Deleted":   3,
	"LockedOut": 4,
}

type Collector struct {
	abstract.BaseCollector

	logger *slog.Logger

	currentDesc         *prometheus.Desc
	totalDesc           *prometheus.Desc
	statusDesc          *prometheus.Desc
	assignmentErrorDesc *prometheus.Desc
}

func NewCollector(logger *slog.Logger, tenant string, msGraphClient *msgraphsdk.GraphServiceClient) *Collector {
	return &Collector{
		BaseCollector: abstract.NewBaseCollector(msGraphClient, subsystem),
		logger:        logger.With(slog.String("collector", subsystem)),

		currentDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "current"),
			"current amount of licenses",
			[]string{subsystem},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		totalDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "total"),
			"total amount of licenses",
			[]string{subsystem, "status"},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		statusDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "status"),
			"status of licenses",
			[]string{subsystem},
			prometheus.Labels{
				"tenant": tenant,
			},
		),
		assignmentErrorDesc: prometheus.NewDesc(
			prometheus.BuildFQName(abstract.Namespace, subsystem, "group_errors"),
			"groups with assignment errors",
			[]string{"group_name", "group_id", subsystem},
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

	ch <- c.currentDesc

	ch <- c.totalDesc

	ch <- c.statusDesc
}

func (c *Collector) ScrapeMetrics(ctx context.Context) ([]prometheus.Metric, error) {
	results, err := c.GraphClient().SubscribedSkus().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("getting SubscribedSkus from the Graph API was not successful: %w", util.GetOdataError(err))
	}

	configuration := &graphgroups.GroupsRequestBuilderGetRequestConfiguration{
		Headers: abstractions.NewRequestHeaders(),
		QueryParameters: &graphgroups.GroupsRequestBuilderGetQueryParameters{
			Count:  to.Ptr(false),
			Filter: to.Ptr("hasMembersWithLicenseErrors eq true"),
			Select: []string{"id", "displayName", "assignedLicenses"},
		},
	}

	configuration.Headers.Add("ConsistencyLevel", "eventual")

	groups, err := c.GraphClient().Groups().Get(ctx, configuration)
	if err != nil {
		return nil, fmt.Errorf("getting SubscribedSkus from the Graph API was not successful: %w", util.GetOdataError(err))
	}

	resultValues := results.GetValue()
	groupsResultValues := groups.GetValue()

	metrics := make([]prometheus.Metric, 0, len(resultValues)*5+len(resultValues)*len(groupsResultValues))

	var (
		ok               bool
		capabilityStatus float64
	)

	for _, result := range resultValues {
		if capabilityStatus, ok = capabilityStatuses[*result.GetCapabilityStatus()]; !ok {
			capabilityStatus = -1
		}

		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.statusDesc,
			prometheus.GaugeValue,
			capabilityStatus,
			*result.GetSkuPartNumber(),
		))

		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.currentDesc,
			prometheus.GaugeValue,
			float64(*result.GetConsumedUnits()),
			*result.GetSkuPartNumber(),
		))

		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.totalDesc,
			prometheus.GaugeValue,
			float64(*result.GetPrepaidUnits().GetEnabled()),
			*result.GetSkuPartNumber(),
			"enabled",
		))

		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.totalDesc,
			prometheus.GaugeValue,
			float64(*result.GetPrepaidUnits().GetWarning()),
			*result.GetSkuPartNumber(),
			"warning",
		))

		metrics = append(metrics, prometheus.MustNewConstMetric(
			c.totalDesc,
			prometheus.GaugeValue,
			float64(*result.GetPrepaidUnits().GetSuspended()),
			*result.GetSkuPartNumber(),
			"suspended",
		))

		for _, group := range groupsResultValues {
			for _, license := range group.GetAssignedLicenses() {
				if license.GetSkuId() != result.GetSkuId() {
					continue
				}

				displayName := group.GetDisplayName()
				skuID := group.GetId()

				if displayName == nil || skuID == nil {
					c.logger.WarnContext(ctx, "When getting groups with license assignment issues, either display name or skuid was nil")

					continue
				}

				metrics = append(metrics, prometheus.MustNewConstMetric(
					c.assignmentErrorDesc,
					prometheus.GaugeValue,
					1,
					*displayName,
					*skuID,
					*result.GetSkuPartNumber(),
				))
			}
		}
	}

	return metrics, nil
}
