package hunting

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/spf13/viper"
)

const (
	subsystem = "hunting"

	labelTenant = "tenant"

	DefaultInterval = time.Hour
	DefaultTimeout  = 2 * time.Minute

	nameQuerySuccess     = "query_success"
	nameQueryResults     = "query_results"
	nameQueryDuration    = "query_duration_seconds"
	nameQueryLastSuccess = "query_last_success_timestamp_seconds"

	fieldKindString = "string"
	fieldKindID     = "id"
	fieldKindValue  = "value"
	fieldKindIgnore = "ignore"
	fieldKindBool   = "bool"

	labelQuery = "query"
)

// Settings is the hunting.query list from configuration.
// enabled and interval are read separately so environment overrides apply.
type Settings struct {
	Queries []Query `mapstructure:"queries"`
}

// Query is one advanced-hunting query and the gauges derived from its rows.
type Query struct {
	Metric       string            `mapstructure:"metric"`
	Help         string            `mapstructure:"help"`
	Query        string            `mapstructure:"query"`
	Timespan     string            `mapstructure:"timespan"`
	WorkspaceID  string            `mapstructure:"workspaceId"`
	Value        *float64          `mapstructure:"value"`
	Labels       map[string]string `mapstructure:"labels"`
	Fields       []Field           `mapstructure:"fields"`
	DefaultField *Field            `mapstructure:"defaultField"`
	Timeout      string            `mapstructure:"timeout"`

	timeout time.Duration
}

// Field maps one result column onto a label or a gauge value.
type Field struct {
	Name    string            `mapstructure:"name"`
	Metric  string            `mapstructure:"metric"`
	Target  string            `mapstructure:"target"`
	Type    string            `mapstructure:"type"`
	Labels  map[string]string `mapstructure:"labels"`
	Filters []Filter          `mapstructure:"filters"`
}

// Filter rewrites a column before it becomes a label or a gauge value.
type Filter struct {
	Type        string `mapstructure:"type"`
	Regexp      string `mapstructure:"regexp"`
	Replacement string `mapstructure:"replacement"`

	re *regexp.Regexp
}

// TimeoutResolved returns the per-query HTTP timeout, including the default.
func (q Query) TimeoutResolved() time.Duration {
	if q.timeout > 0 {
		return q.timeout
	}

	return DefaultTimeout
}

// ParseInterval parses hunting.interval. An empty string uses the default.
func ParseInterval(raw string) (time.Duration, error) {
	if raw == "" {
		return DefaultInterval, nil
	}

	interval, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse hunting interval: %w", err)
	}

	if interval <= 0 {
		return 0, errors.New("hunting interval must be positive")
	}

	return interval, nil
}

// LoadSettings reads hunting.queries from viper.
// Call it only when hunting is enabled; invalid queries are ignored while the collector is off.
func LoadSettings() (Settings, error) {
	var settings Settings

	err := viper.UnmarshalKey("hunting", &settings)
	if err != nil {
		return Settings{}, fmt.Errorf("parse hunting configuration: %w", err)
	}

	return settings, nil
}

// Prepare validates queries and compiles filters.
// It returns an error when the list is empty, so callers must skip it while hunting is disabled.
func Prepare(settings Settings) (Settings, error) {
	if len(settings.Queries) == 0 {
		return Settings{}, errors.New("hunting.enabled requires at least one query")
	}

	owners := map[string]int{}

	for i := range settings.Queries {
		err := prepareQuery(&settings.Queries[i], i, owners)
		if err != nil {
			return Settings{}, err
		}
	}

	return settings, nil
}

func prepareQuery(query *Query, queryIndex int, owners map[string]int) error {
	err := claimMetric(owners, queryIndex, query.Metric)
	if err != nil {
		return err
	}

	if strings.TrimSpace(query.Query) == "" {
		return fmt.Errorf("query %q: missing query", query.Metric)
	}

	err = validateWorkspace(query)
	if err != nil {
		return err
	}

	err = prepareTimeout(query)
	if err != nil {
		return err
	}

	if query.DefaultField != nil && fieldType(query.DefaultField.Type) != fieldKindIgnore {
		return fmt.Errorf("query %q: defaultField must be ignore", query.Metric)
	}

	err = validateQueryLabels(query)
	if err != nil {
		return err
	}

	err = prepareFields(query, queryIndex, owners)
	if err != nil {
		return err
	}

	if !canEmit(*query) {
		return fmt.Errorf("query %q: cannot emit a metric value", query.Metric)
	}

	return nil
}

func validateWorkspace(query *Query) error {
	if query.WorkspaceID == "" {
		return nil
	}

	_, err := uuid.Parse(query.WorkspaceID)
	if err != nil {
		return fmt.Errorf("query %q: workspaceId: %w", query.Metric, err)
	}

	return nil
}

func validateQueryLabels(query *Query) error {
	for name := range query.Labels {
		err := validateLabelName(name)
		if err != nil {
			return fmt.Errorf("query %q: %w", query.Metric, err)
		}
	}

	return nil
}

func prepareTimeout(query *Query) error {
	if query.Timeout == "" {
		query.timeout = DefaultTimeout

		return nil
	}

	timeout, err := time.ParseDuration(query.Timeout)
	if err != nil {
		return fmt.Errorf("query %q: timeout: %w", query.Metric, err)
	}

	if timeout <= 0 {
		return fmt.Errorf("query %q: timeout must be positive", query.Metric)
	}

	query.timeout = timeout

	return nil
}

func prepareFields(query *Query, queryIndex int, owners map[string]int) error {
	parentValues := 0

	for i := range query.Fields {
		valueOnParent, err := prepareField(query, &query.Fields[i], queryIndex, owners)
		if err != nil {
			return err
		}

		if valueOnParent {
			parentValues++
		}
	}

	if parentValues > 1 {
		return fmt.Errorf("query %q: more than one value field writes the parent metric", query.Metric)
	}

	return validateLabelPlacement(*query)
}

func prepareField(query *Query, field *Field, queryIndex int, owners map[string]int) (bool, error) {
	kind, err := validateFieldDefinition(query, field)
	if err != nil {
		return false, err
	}

	if field.Metric != "" {
		err = claimMetric(owners, queryIndex, field.Metric)
		if err != nil {
			return false, err
		}
	}

	filters, err := prepareFilters(field.Filters)
	if err != nil {
		return false, fmt.Errorf("query %q: field %q: %w", query.Metric, field.Name, err)
	}

	field.Filters = filters

	return kind == fieldKindValue && field.Metric == "", nil
}

func validateFieldDefinition(query *Query, field *Field) (string, error) {
	if field.Name == "" {
		return "", fmt.Errorf("query %q: field name is required", query.Metric)
	}

	kind := fieldType(field.Type)
	if !knownFieldType(kind) {
		return "", fmt.Errorf("query %q: field %q: unknown type %q", query.Metric, field.Name, field.Type)
	}

	if field.Metric != "" && kind != fieldKindValue {
		return "", fmt.Errorf("query %q: field %q: metric requires type value", query.Metric, field.Name)
	}

	if field.Metric == "" && len(field.Labels) > 0 {
		return "", fmt.Errorf("query %q: field %q: labels require metric", query.Metric, field.Name)
	}

	return kind, nil
}

func prepareFilters(filters []Filter) ([]Filter, error) {
	prepared := make([]Filter, len(filters))

	for i, filter := range filters {
		switch strings.ToLower(filter.Type) {
		case "tolower", "toupper", "tounixtime":
		case "regexp":
			if filter.Regexp == "" {
				return nil, errors.New("regexp filter missing regexp")
			}

			compiled, err := regexp.Compile(filter.Regexp)
			if err != nil {
				return nil, fmt.Errorf("compile regexp: %w", err)
			}

			filter.re = compiled
		default:
			return nil, fmt.Errorf("unsupported filter %q", filter.Type)
		}

		prepared[i] = filter
	}

	return prepared, nil
}

func canEmit(query Query) bool {
	if query.Value != nil {
		return true
	}

	for _, field := range query.Fields {
		if fieldType(field.Type) == fieldKindValue {
			return true
		}
	}

	return false
}

func validateLabelPlacement(query Query) error {
	idNames, err := idLabelNames(query)
	if err != nil {
		return err
	}

	err = validateParentLabels(query, idNames)
	if err != nil {
		return err
	}

	for _, field := range query.Fields {
		if field.Metric == "" {
			continue
		}

		err = validateFieldSampleLabels(query, field, idNames)
		if err != nil {
			return err
		}
	}

	return nil
}

func idLabelNames(query Query) ([]string, error) {
	names := make([]string, 0)
	seen := map[string]struct{}{}

	for _, field := range query.Fields {
		if fieldType(field.Type) != fieldKindID {
			continue
		}

		name := fieldLabelName(field)

		err := validateLabelName(name)
		if err != nil {
			return nil, fmt.Errorf("query %q: %w", query.Metric, err)
		}

		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("query %q: duplicate label %q", query.Metric, name)
		}

		seen[name] = struct{}{}

		names = append(names, name)
	}

	return names, nil
}

func validateParentLabels(query Query, idNames []string) error {
	seen := map[string]struct{}{}

	err := addLabelSet(seen, query, mapKeys(query.Labels))
	if err != nil {
		return err
	}

	err = addLabelSet(seen, query, idNames)
	if err != nil {
		return err
	}

	return addLabelSet(seen, query, parentFieldLabels(query))
}

func parentFieldLabels(query Query) []string {
	names := make([]string, 0)

	for _, field := range query.Fields {
		kind := fieldType(field.Type)
		if field.Metric != "" || kind == fieldKindID || !isLabelType(kind) {
			continue
		}

		names = append(names, fieldLabelName(field))
	}

	return names
}

func addLabelSet(seen map[string]struct{}, query Query, names []string) error {
	for _, name := range names {
		err := validateLabelName(name)
		if err != nil {
			return fmt.Errorf("query %q: %w", query.Metric, err)
		}

		if _, ok := seen[name]; ok {
			return fmt.Errorf("query %q: duplicate label %q", query.Metric, name)
		}

		seen[name] = struct{}{}
	}

	return nil
}

func mapKeys(labels map[string]string) []string {
	names := make([]string, 0, len(labels))

	for name := range labels {
		names = append(names, name)
	}

	return names
}

func validateFieldSampleLabels(query Query, field Field, idNames []string) error {
	seen := map[string]struct{}{}

	for _, name := range idNames {
		seen[name] = struct{}{}
	}

	return addLabelSet(seen, query, mapKeys(field.Labels))
}

func claimMetric(owners map[string]int, queryIndex int, name string) error {
	err := validateMetricName(name)
	if err != nil {
		return err
	}

	owner, ok := owners[name]
	if ok && owner != queryIndex {
		return fmt.Errorf("metric %q is used by more than one hunting query", name)
	}

	owners[name] = queryIndex

	return nil
}

func validateMetricName(name string) error {
	if strings.HasPrefix(name, "__") || !model.LegacyValidation.IsValidMetricName(name) {
		return fmt.Errorf("invalid prometheus metric name %q", name)
	}

	if isReservedMetric(name) {
		return fmt.Errorf("metric name %q is reserved", name)
	}

	return nil
}

func validateLabelName(name string) error {
	if name == labelTenant || strings.HasPrefix(name, "__") || !model.LegacyValidation.IsValidLabelName(name) {
		return fmt.Errorf("invalid prometheus label name %q", name)
	}

	return nil
}

func isReservedMetric(name string) bool {
	switch name {
	case operationalName(nameQuerySuccess), operationalName(nameQueryResults), operationalName(nameQueryDuration), operationalName(nameQueryLastSuccess):
		return true
	default:
		return false
	}
}

func operationalName(name string) string {
	return prometheus.BuildFQName(abstract.Namespace, subsystem, name)
}

func fieldType(raw string) string {
	switch strings.ToLower(raw) {
	case "", fieldKindString:
		return fieldKindString
	case "boolean":
		return fieldKindBool
	default:
		return strings.ToLower(raw)
	}
}

func knownFieldType(kind string) bool {
	switch kind {
	case fieldKindString, fieldKindID, fieldKindValue, fieldKindIgnore, fieldKindBool:
		return true
	default:
		return false
	}
}

func isLabelType(kind string) bool {
	switch kind {
	case fieldKindString, fieldKindID, fieldKindBool:
		return true
	default:
		return false
	}
}

func fieldLabelName(field Field) string {
	if field.Target != "" {
		return field.Target
	}

	return field.Name
}

func metricNames(q Query) []string {
	names := map[string]struct{}{q.Metric: {}}

	for _, field := range q.Fields {
		if field.Metric != "" {
			names[field.Metric] = struct{}{}
		}
	}

	return sortedKeys(names)
}

func variableLabels(query Query, metricName string) []string {
	names := map[string]struct{}{}
	addIDLabels(names, query)

	if metricName == query.Metric {
		addParentLabels(names, query)
	}

	addPublishingLabels(names, query, metricName)

	return sortedKeys(names)
}

func addIDLabels(names map[string]struct{}, query Query) {
	for _, field := range query.Fields {
		if fieldType(field.Type) == fieldKindID {
			names[fieldLabelName(field)] = struct{}{}
		}
	}
}

func addParentLabels(names map[string]struct{}, query Query) {
	for name := range query.Labels {
		names[name] = struct{}{}
	}

	for _, name := range parentFieldLabels(query) {
		names[name] = struct{}{}
	}
}

func addPublishingLabels(names map[string]struct{}, query Query, metricName string) {
	for _, field := range query.Fields {
		if field.Metric != metricName {
			continue
		}

		for name := range field.Labels {
			names[name] = struct{}{}
		}
	}
}

func metricHelp(q Query, name string) string {
	if name == q.Metric && q.Help != "" {
		return q.Help
	}

	return name
}

func sortedKeys(names map[string]struct{}) []string {
	keys := make([]string, 0, len(names))

	for name := range names {
		keys = append(keys, name)
	}

	slices.Sort(keys)

	return keys
}
