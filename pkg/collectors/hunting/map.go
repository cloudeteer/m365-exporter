package hunting

import (
	"encoding/json"
	"maps"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Sample is one gauge produced from a hunting result row.
// Labels are the variable labels. The tenant const label is added by the collector.
type Sample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// MapRow converts one result object into gauge samples.
// Samples are ordered so later fields overwrite earlier ones when the collector deduplicates them.
// The returned count is the number of values that were dropped, including non-numeric strings and nested columns.
func MapRow(query Query, row map[string]any) ([]Sample, int) {
	idLabels, dropped := idLabelsFromRow(query, row)
	parent, hasParent := newParentSample(query)
	samples := make([]Sample, 0)

	for _, field := range query.Fields {
		fieldSamples, fieldDropped, parentValue, ok := mapField(field, row, idLabels)
		dropped += fieldDropped

		if !ok {
			continue
		}

		if parentValue != nil {
			parent.Value = *parentValue
			hasParent = true

			continue
		}

		if fieldSamples == nil {
			continue
		}

		if fieldSamples.Name != "" {
			samples = append(samples, *fieldSamples)

			continue
		}

		maps.Copy(parent.Labels, fieldSamples.Labels)
	}

	if !hasParent {
		return samples, dropped
	}

	maps.Copy(parent.Labels, idLabels)

	return append([]Sample{parent}, samples...), dropped
}

func idLabelsFromRow(query Query, row map[string]any) (map[string]string, int) {
	labels := map[string]string{}
	dropped := 0

	for _, field := range query.Fields {
		if fieldType(field.Type) != fieldKindID {
			continue
		}

		text, ok, skip := columnText(field, row)
		if skip {
			continue
		}

		if !ok {
			dropped++

			continue
		}

		labels[fieldLabelName(field)] = text
	}

	return labels, dropped
}

func newParentSample(query Query) (Sample, bool) {
	parent := Sample{
		Name:   query.Metric,
		Labels: cloneLabels(query.Labels),
	}

	if query.Value == nil {
		return parent, false
	}

	parent.Value = *query.Value

	return parent, true
}

func mapField(field Field, row map[string]any, idLabels map[string]string) (*Sample, int, *float64, bool) {
	kind := fieldType(field.Type)
	if kind == fieldKindIgnore || kind == fieldKindID {
		return nil, 0, nil, false
	}

	raw, exists := row[field.Name]
	if !exists {
		return nil, 0, nil, false
	}

	if isNested(raw) {
		return nil, 1, nil, false
	}

	if field.Metric != "" {
		return metricSample(field, raw, idLabels)
	}

	if kind == fieldKindValue {
		value, ok := metricValue(raw, field)
		if !ok {
			return nil, 1, nil, false
		}

		return nil, 0, &value, true
	}

	text, ok := labelText(raw, field)
	if !ok {
		return nil, 1, nil, false
	}

	return &Sample{Labels: map[string]string{fieldLabelName(field): text}}, 0, nil, true
}

func columnText(field Field, row map[string]any) (string, bool, bool) {
	raw, exists := row[field.Name]
	if !exists {
		return "", false, true
	}

	if isNested(raw) {
		return "", false, false
	}

	text, ok := labelText(raw, field)

	return text, ok, false
}

func metricSample(field Field, raw any, idLabels map[string]string) (*Sample, int, *float64, bool) {
	value, ok := metricValue(raw, field)
	if !ok {
		return nil, 1, nil, false
	}

	labels := cloneLabels(field.Labels)
	maps.Copy(labels, idLabels)

	return &Sample{Name: field.Metric, Labels: labels, Value: value}, 0, nil, true
}

func metricValue(raw any, field Field) (float64, bool) {
	if raw == nil {
		return 0, false
	}

	if number, ok := asNumber(raw); ok {
		return number, true
	}

	if value, ok := raw.(bool); ok {
		if value {
			return 1, true
		}

		return 0, true
	}

	text, ok := raw.(string)
	if !ok {
		return 0, false
	}

	parsed, err := strconv.ParseFloat(applyFilters(text, field.Filters), 64)
	if err != nil {
		return 0, false
	}

	return parsed, true
}

func labelText(raw any, field Field) (string, bool) {
	text, ok := asString(raw)
	if !ok {
		return "", false
	}

	if fieldType(field.Type) == fieldKindBool {
		switch strings.ToLower(text) {
		case "1", "true", "yes":
			text = "true"
		default:
			text = "false"
		}
	}

	return applyFilters(text, field.Filters), true
}

func applyFilters(value string, filters []Filter) string {
	for _, filter := range filters {
		switch strings.ToLower(filter.Type) {
		case "tolower":
			value = strings.ToLower(value)
		case "toupper":
			value = strings.ToUpper(value)
		case "tounixtime":
			value = toUnix(value)
		case "regexp":
			compiled := filter.re
			if compiled == nil && filter.Regexp != "" {
				var err error

				compiled, err = regexp.Compile(filter.Regexp)
				if err != nil {
					continue
				}
			}

			if compiled != nil {
				value = compiled.ReplaceAllString(value, filter.Replacement)
			}
		}
	}

	return value
}

func toUnix(value string) string {
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05 +07:00",
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05",
		time.RFC822,
		time.RFC822Z,
		time.RFC850,
		time.RFC1123,
		time.RFC1123Z,
		time.RFC3339Nano,
	}

	for _, format := range formats {
		parsed, err := time.Parse(format, value)
		if err == nil && parsed.Unix() > 0 {
			return strconv.FormatInt(parsed.Unix(), 10)
		}
	}

	return ""
}

func asNumber(raw any) (float64, bool) {
	switch value := raw.(type) {
	case json.Number:
		parsed, err := value.Float64()

		return parsed, err == nil
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case int32:
		return float64(value), true
	default:
		return 0, false
	}
}

func asString(raw any) (string, bool) {
	switch value := raw.(type) {
	case string:
		return value, true
	case json.Number:
		return value.String(), true
	case bool:
		return strconv.FormatBool(value), true
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), true
	case int:
		return strconv.Itoa(value), true
	case int64:
		return strconv.FormatInt(value, 10), true
	case nil:
		return "", true
	default:
		return "", false
	}
}

func isNested(raw any) bool {
	switch raw.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

func cloneLabels(labels map[string]string) map[string]string {
	cloned := make(map[string]string, len(labels))
	maps.Copy(cloned, labels)

	return cloned
}
