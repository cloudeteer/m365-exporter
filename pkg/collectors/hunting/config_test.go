package hunting_test

import (
	"strings"
	"testing"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/hunting"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		queries []hunting.Query
		want    string
	}{
		{
			name: "zero queries",
			want: "at least one query",
		},
		{
			name: "legacy metric name",
			queries: []hunting.Query{{
				Metric: "m365.hunting",
				Query:  "DeviceInfo",
				Value:  floatPtr(1),
			}},
			want: "invalid prometheus metric name",
		},
		{
			name: "reserved metric name",
			queries: []hunting.Query{{
				Metric: "m365_hunting_query_success",
				Query:  "DeviceInfo",
				Value:  floatPtr(1),
			}},
			want: "reserved",
		},
		{
			name: "underscore prefix",
			queries: []hunting.Query{{
				Metric: "__hunting",
				Query:  "DeviceInfo",
				Value:  floatPtr(1),
			}},
			want: "invalid prometheus metric name",
		},
		{
			name: "missing query text",
			queries: []hunting.Query{{
				Metric: "m365_hunting_devices",
				Value:  floatPtr(1),
			}},
			want: "missing query",
		},
		{
			name: "tenant query label",
			queries: []hunting.Query{{
				Metric: "m365_hunting_devices",
				Query:  "DeviceInfo",
				Value:  floatPtr(1),
				Labels: map[string]string{"tenant": "x"},
			}},
			want: "invalid prometheus label name",
		},
		{
			name: "tenant field label",
			queries: []hunting.Query{{
				Metric: "m365_hunting_devices",
				Query:  "DeviceInfo",
				Fields: []hunting.Field{{
					Name:   "count_",
					Type:   "value",
					Metric: "m365_hunting_device_count",
					Labels: map[string]string{"tenant": "x"},
				}},
			}},
			want: "invalid prometheus label name",
		},
		{
			name: "duplicate metric across queries",
			queries: []hunting.Query{
				{Metric: "m365_hunting_devices", Query: "DeviceInfo", Value: floatPtr(1)},
				{Metric: "m365_hunting_devices", Query: "DeviceInfo | take 1", Value: floatPtr(1)},
			},
			want: "more than one hunting query",
		},
		{
			name: "field metric collides with another query",
			queries: []hunting.Query{
				{Metric: "m365_hunting_devices", Query: "DeviceInfo", Value: floatPtr(1)},
				{
					Metric: "m365_hunting_other",
					Query:  "DeviceInfo",
					Fields: []hunting.Field{{
						Name:   "count_",
						Type:   "value",
						Metric: "m365_hunting_devices",
					}},
				},
			},
			want: "more than one hunting query",
		},
		{
			name: "default field string",
			queries: []hunting.Query{{
				Metric:       "m365_hunting_devices",
				Query:        "DeviceInfo",
				Value:        floatPtr(1),
				DefaultField: &hunting.Field{Type: "string"},
			}},
			want: "defaultField must be ignore",
		},
		{
			name: "bad regexp",
			queries: []hunting.Query{{
				Metric: "m365_hunting_devices",
				Query:  "DeviceInfo",
				Fields: []hunting.Field{{
					Name:   "status",
					Type:   "value",
					Metric: "m365_hunting_status",
					Filters: []hunting.Filter{{
						Type:   "regexp",
						Regexp: "(",
					}},
				}},
			}},
			want: "regexp",
		},
		{
			name: "no value source",
			queries: []hunting.Query{{
				Metric: "m365_hunting_devices",
				Query:  "DeviceInfo",
				Fields: []hunting.Field{{Name: "OSPlatform", Type: "id"}},
			}},
			want: "cannot emit",
		},
		{
			name: "two parent value fields",
			queries: []hunting.Query{{
				Metric: "m365_hunting_devices",
				Query:  "DeviceInfo",
				Fields: []hunting.Field{
					{Name: "a", Type: "value"},
					{Name: "b", Type: "value"},
				},
			}},
			want: "more than one value field",
		},
		{
			name: "id field with metric",
			queries: []hunting.Query{{
				Metric: "m365_hunting_devices",
				Query:  "DeviceInfo",
				Fields: []hunting.Field{{
					Name:   "DeviceId",
					Type:   "id",
					Metric: "m365_hunting_device_info",
				}},
			}},
			want: "type value",
		},
		{
			name: "labels without metric",
			queries: []hunting.Query{{
				Metric: "m365_hunting_devices",
				Query:  "DeviceInfo",
				Value:  floatPtr(1),
				Fields: []hunting.Field{{
					Name:   "OSPlatform",
					Type:   "id",
					Labels: map[string]string{"kind": "os"},
				}},
			}},
			want: "labels require metric",
		},
		{
			name: "id label collides with field label",
			queries: []hunting.Query{{
				Metric: "m365_hunting_devices",
				Query:  "DeviceInfo",
				Fields: []hunting.Field{
					{Name: "kind", Type: "id"},
					{
						Name:   "status",
						Type:   "value",
						Metric: "m365_hunting_status",
						Labels: map[string]string{"kind": "assessment"},
					},
				},
			}},
			want: "duplicate label",
		},
		{
			name: "bad workspace id",
			queries: []hunting.Query{{
				Metric:      "m365_hunting_devices",
				Query:       "DeviceInfo",
				Value:       floatPtr(1),
				WorkspaceID: "not-a-uuid",
			}},
			want: "workspaceId",
		},
		{
			name: "bad timeout",
			queries: []hunting.Query{{
				Metric:  "m365_hunting_devices",
				Query:   "DeviceInfo",
				Value:   floatPtr(1),
				Timeout: "soon",
			}},
			want: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := hunting.Prepare(hunting.Settings{Queries: tt.queries})
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestPrepareAcceptsSharedMetricName(t *testing.T) {
	t.Parallel()

	settings, err := hunting.Prepare(hunting.Settings{Queries: []hunting.Query{{
		Metric: "m365_hunting_device_patch",
		Query:  "DeviceInfo",
		Fields: []hunting.Field{
			{Name: "DeviceId", Target: "device_id", Type: "id"},
			{
				Name:    "AssessmentStatus",
				Type:    "value",
				Metric:  "m365_hunting_device_patch_status",
				Labels:  map[string]string{"kind": "assessment"},
				Filters: []hunting.Filter{{Type: "regexp", Regexp: "^Succeeded$", Replacement: "1"}},
			},
			{
				Name:   "PatchStatus",
				Type:   "value",
				Metric: "m365_hunting_device_patch_status",
				Labels: map[string]string{"kind": "patch"},
			},
		},
	}}})
	require.NoError(t, err)
	require.Len(t, settings.Queries, 1)
	assert.Equal(t, hunting.DefaultTimeout, settings.Queries[0].TimeoutResolved())
}

func TestLoadSettingsLowercasesLabelKeys(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.SetConfigType("yaml")

	err := viper.ReadConfig(strings.NewReader(`
hunting:
  queries:
    - metric: m365_hunting_devices
      query: DeviceInfo
      value: 1
      labels:
        DeviceKind: laptop
`))
	require.NoError(t, err)

	settings, err := hunting.LoadSettings()
	require.NoError(t, err)
	require.Len(t, settings.Queries, 1)
	assert.Equal(t, "laptop", settings.Queries[0].Labels["devicekind"])
	assert.Equal(t, "DeviceInfo", settings.Queries[0].Query)
}

func TestParseInterval(t *testing.T) {
	t.Parallel()

	interval, err := hunting.ParseInterval("")
	require.NoError(t, err)
	assert.Equal(t, hunting.DefaultInterval, interval)

	interval, err = hunting.ParseInterval("15m")
	require.NoError(t, err)
	assert.Equal(t, hunting.DefaultInterval/4, interval)

	_, err = hunting.ParseInterval("nope")
	require.Error(t, err)
}

func floatPtr(v float64) *float64 {
	return &v
}
