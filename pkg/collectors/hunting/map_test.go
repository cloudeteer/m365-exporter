package hunting_test

import (
	"encoding/json"
	"testing"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/hunting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapRowPatchQuery(t *testing.T) {
	t.Parallel()

	query := mustPrepareOne(t, hunting.Query{
		Metric: "m365_hunting_device_patch",
		Query:  "DeviceInfo",
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
				Filters: []hunting.Filter{
					{Type: "regexp", Regexp: ".*", Replacement: "0"},
				},
			},
			{
				Name:   "CriticalCount",
				Type:   "value",
				Metric: "m365_hunting_device_patch_count",
				Labels: map[string]string{"classification": "critical"},
			},
		},
	})

	samples, dropped := hunting.MapRow(query, map[string]any{
		"DeviceId":         "dev-1",
		"OSPlatform":       "linux",
		"AssessmentStatus": "Succeeded",
		"PatchStatus":      "Ready",
		"SecurityCount":    json.Number("4"),
		"CriticalCount":    json.Number("2"),
		"Timestamp":        "2024-03-26T09:39:50Z",
	})
	require.Equal(t, 0, dropped)
	require.Len(t, samples, 4)

	byKey := map[string]hunting.Sample{}
	for _, sample := range samples {
		byKey[sample.Name+"/"+sample.Labels["kind"]+sample.Labels["classification"]] = sample
		assert.Equal(t, "dev-1", sample.Labels["device_id"])
		assert.Equal(t, "linux", sample.Labels["os_platform"])
		assert.NotContains(t, sample.Labels, "tenant")
		assert.NotContains(t, sample.Labels, "Timestamp")
	}

	assert.Equal(t, 1.0, byKey["m365_hunting_device_patch_status/assessment"].Value)
	assert.Equal(t, 1.0, byKey["m365_hunting_device_patch_status/patch"].Value)
	assert.Equal(t, 4.0, byKey["m365_hunting_device_patch_count/security"].Value)
	assert.Equal(t, 2.0, byKey["m365_hunting_device_patch_count/critical"].Value)
	assert.NotContains(t, byKey, "m365_hunting_device_patch/")
}

func TestMapRowDropsNonNumericString(t *testing.T) {
	t.Parallel()

	query := mustPrepareOne(t, hunting.Query{
		Metric: "m365_hunting_status",
		Query:  "DeviceInfo",
		Fields: []hunting.Field{
			{Name: "status", Type: "value", Metric: "m365_hunting_status_value"},
			{Name: "count_", Type: "value"},
		},
	})

	samples, dropped := hunting.MapRow(query, map[string]any{
		"status": "nope",
		"count_": json.Number("3"),
	})
	require.Equal(t, 1, dropped)
	require.Len(t, samples, 1)
	assert.Equal(t, "m365_hunting_status", samples[0].Name)
	assert.Equal(t, 3.0, samples[0].Value)
}

func TestMapRowStaticValueAndBoolLabel(t *testing.T) {
	t.Parallel()

	query := mustPrepareOne(t, hunting.Query{
		Metric: "m365_hunting_info",
		Query:  "DeviceInfo",
		Value:  floatPtr(1),
		Labels: map[string]string{"source": "defender"},
		Fields: []hunting.Field{
			{Name: "enabled", Type: "bool"},
			{Name: "secret", Type: "ignore"},
			{Name: "OSPlatform", Target: "os_platform", Type: "id"},
		},
	})

	samples, dropped := hunting.MapRow(query, map[string]any{
		"enabled":    true,
		"secret":     "hidden",
		"OSPlatform": "Windows",
		"extra":      map[string]any{"nested": true},
	})
	require.Equal(t, 0, dropped)
	require.Len(t, samples, 1)
	assert.Equal(t, 1.0, samples[0].Value)
	assert.Equal(t, "true", samples[0].Labels["enabled"])
	assert.Equal(t, "Windows", samples[0].Labels["os_platform"])
	assert.Equal(t, "defender", samples[0].Labels["source"])
	assert.NotContains(t, samples[0].Labels, "secret")
	assert.NotContains(t, samples[0].Labels, "extra")
}

func mustPrepareOne(t *testing.T, query hunting.Query) hunting.Query {
	t.Helper()

	settings, err := hunting.Prepare(hunting.Settings{Queries: []hunting.Query{query}})
	require.NoError(t, err)

	return settings.Queries[0]
}
