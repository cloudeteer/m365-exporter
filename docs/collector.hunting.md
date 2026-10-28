# hunting collector

The hunting collector runs [advanced hunting](https://learn.microsoft.com/en-us/graph/api/security-security-runhuntingquery) queries that you define, and turns each result row into Prometheus gauges.

It is off unless `hunting.enabled` is true. The other collectors do not change.

The app registration needs the application permission `ThreatHunting.Read.All`.

## Configuration

Queries are a list. `hunting.interval` is how often they run (default `1h`). Each query can set `timeout` (default `2m`), `timespan`, and `workspaceId`.

If `timespan` is omitted, Microsoft Graph looks back 30 days. Prefer a shorter `timespan`. Bound the result in the query itself, for example with `| take` or `summarize`. The collector reads the full response into memory. A large result can exhaust memory and stop the process, including the other collectors.

`name` is the result column and must match it exactly, including capitalization. `target` is the Prometheus label for that column. `labels` add a fixed label to a field that sets its own `metric`.

Filters are objects (`type`, and for `regexp` a `regexp` plus `replacement`). `toLower`, `toUpper`, `toUnixtime`, and `regexp` are supported. A numeric value used as a gauge does not pass through string filters. A string value does, and is then parsed as a number.

`defaultField` can only be `ignore`, which is the default. Columns you do not list are dropped, so count and status columns must be listed. Several fields may publish the same metric name. Give them different `labels`, such as `severity`, to keep the series apart. `type: id` labels are copied onto every series from that row.

```yaml
hunting:
  enabled: true
  interval: 1h
  queries:
    - metric: m365_hunting_device_vulnerability
      timeout: 2m
      query: |-
        DeviceTvmSoftwareVulnerabilities
        | summarize
            CriticalCount = countif(VulnerabilitySeverityLevel == "Critical"),
            HighCount = countif(VulnerabilitySeverityLevel == "High")
          by DeviceId, DeviceName, OSPlatform
      fields:
        - name: DeviceId
          target: device_id
          type: id
        - name: DeviceName
          target: device_name
          type: id
        - name: OSPlatform
          target: os_platform
          type: id
        - name: CriticalCount
          metric: m365_hunting_device_vulnerability_count
          type: value
          labels:
            severity: critical
        - name: HighCount
          metric: m365_hunting_device_vulnerability_count
          type: value
          labels:
            severity: high
```

`DeviceInfo` has no patch or vulnerability columns. `DeviceTvmSoftwareVulnerabilities` lists one row per CVE, so the query counts critical and high findings per device. `DeviceName` is that device's fully qualified domain name. Both counts publish `m365_hunting_device_vulnerability_count`. The `severity` label keeps the two series apart.

One device with 2 critical and 4 high findings becomes:

```text
m365_hunting_device_vulnerability_count{device_id="9eaf3a8b5962e0e6b1af9ec756664a9b823df2d1",device_name="laptop.contoso.com",os_platform="Windows 11",severity="critical",tenant="contoso"} 2
m365_hunting_device_vulnerability_count{device_id="9eaf3a8b5962e0e6b1af9ec756664a9b823df2d1",device_name="laptop.contoso.com",os_platform="Windows 11",severity="high",tenant="contoso"} 4
```

`m365_hunting_device_vulnerability` itself is absent, because that query has no value of its own. Configured metrics include the `tenant` label. The `m365_hunting_query_*` gauges only have the `query` label. High-cardinality columns, such as device IDs, multiply the number of series by the number of rows.

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
