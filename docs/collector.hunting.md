# hunting collector

The hunting collector runs [advanced hunting](https://learn.microsoft.com/en-us/graph/api/security-security-runhuntingquery) queries that you define, and turns each result row into Prometheus gauges.

It is off unless `hunting.enabled` is true. The other collectors do not change.

The app registration needs the application permission `ThreatHunting.Read.All`.

## Configuration

Queries are a list. `hunting.interval` is how often they run (default `1h`). Each query can set `timeout` (default `2m`), `timespan`, `workspaceId`, and `maxRows` (default `1000`).

If `timespan` is omitted, Microsoft Graph looks back 30 days. Prefer a shorter `timespan`, and use `| take` in the query. When a response has more rows than `maxRows`, that query fails and the previous series stay in place.

Viper lowercases label keys. `kind` stays `kind`. `DeviceKind` becomes `devicekind`. Column `name` values are not lowercased and must match the JSON field exactly.

Filters are objects (`type`, and for `regexp` a `regexp` plus `replacement`). `toLower`, `toUpper`, `toUnixtime`, and `regexp` are supported. A numeric value used as a gauge does not pass through string filters. A string value does, and is then parsed as a number.

`defaultField` can only be `ignore`, which is the default. Columns you do not list are dropped, so count and status columns must be listed. Several fields may publish the same metric name. Give them different `labels`, such as `kind`, to keep the series apart. `type: id` labels are copied onto every series from that row.

```yaml
hunting:
  enabled: true
  interval: 1h
  queries:
    - metric: m365_hunting_device_patch
      timespan: P1D
      timeout: 2m
      query: |-
        DeviceInfo
        | project DeviceId, OSPlatform, AssessmentStatus, PatchStatus, SecurityCount, CriticalCount
      fields:
        - name: DeviceId
          target: device_id
          type: id
        - name: OSPlatform
          target: os_platform
          type: id
        - name: AssessmentStatus
          metric: m365_hunting_device_patch_status
          type: value
          labels:
            kind: assessment
          filters:
            - type: regexp
              regexp: "^Succeeded$"
              replacement: "1"
            - type: regexp
              regexp: "^Failed$"
              replacement: "3"
            - type: regexp
              regexp: "^[A-Za-z]+$"
              replacement: "-1"
        - name: PatchStatus
          metric: m365_hunting_device_patch_status
          type: value
          labels:
            kind: patch
          filters:
            - type: regexp
              regexp: "^Ready$"
              replacement: "1"
            - type: regexp
              regexp: "^Failed$"
              replacement: "3"
            - type: regexp
              regexp: "^[A-Za-z]+$"
              replacement: "-1"
        - name: SecurityCount
          metric: m365_hunting_device_patch_count
          type: value
          labels:
            classification: security
        - name: CriticalCount
          metric: m365_hunting_device_patch_count
          type: value
          labels:
            classification: critical
```

One row with `AssessmentStatus` `Succeeded`, `PatchStatus` `Ready`, `SecurityCount` 4, and `CriticalCount` 2 becomes:

```text
m365_hunting_device_patch_status{device_id="dev-1",kind="assessment",os_platform="linux",tenant="contoso"} 1
m365_hunting_device_patch_status{device_id="dev-1",kind="patch",os_platform="linux",tenant="contoso"} 1
m365_hunting_device_patch_count{classification="security",device_id="dev-1",os_platform="linux",tenant="contoso"} 4
m365_hunting_device_patch_count{classification="critical",device_id="dev-1",os_platform="linux",tenant="contoso"} 2
```

`m365_hunting_device_patch` itself is absent, because that query has no value of its own. Configured metrics include the `tenant` label. The `m365_hunting_query_*` gauges only have the `query` label. High-cardinality columns, such as device ids, multiply the number of series by the number of rows.

## Metrics

| Name                                                     | Description                                                                 | Type  | Labels   |
|----------------------------------------------------------|-----------------------------------------------------------------------------|-------|----------|
| `m365_hunting_query_success`                             | 1 when the last run of this query succeeded                                 | Gauge | `query`  |
| `m365_hunting_query_results`                             | Rows returned by the last successful run. 0 when the run failed             | Gauge | `query`  |
| `m365_hunting_query_duration_seconds`                    | Seconds spent on the last run                                               | Gauge | `query`  |
| `m365_hunting_query_last_success_timestamp_seconds`      | Unix time of the last successful run. Unchanged when a later run fails      | Gauge | `query`  |
| configured `metric`                                      | Gauge built from the query. Label names come from the field list            | Gauge | varies, plus `tenant` |

`m365_collector_scrape_success{collector="hunting"}` means the loop finished. A failed query does not clear the other queries. Alert on the per-query gauge.

## Alerting examples

```text
m365_hunting_query_success == 0
```
