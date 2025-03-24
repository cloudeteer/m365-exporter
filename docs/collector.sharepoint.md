# sharepoint collector

The sharepoint collector collects overall sharepoint storage usage (like quota)

## Configuration

None

## Metrics

| Name                             | Description              | Type    | Labels                   |
|----------------------------------|--------------------------|---------|--------------------------|
| `m365_sharepoint_usage_info`     | sharepoint storage usage | Gauge   | `tenant`,`name`,`type`   |

## Example metric

```
# HELP m365_sharepoint_usage_info Sharepoint metrics
# TYPE m365_sharepoint_usage_info gauge
m365_sharepoint_usage_info{name="dummy",tenant="0000000-0000-0000-0000-000000000000",type="GeoAllocatedStorageMB"} 0
m365_sharepoint_usage_info{name="dummy",tenant="0000000-0000-0000-0000-000000000000",type="GeoAvailableStorageMB"} 3.882755e+06
m365_sharepoint_usage_info{name="dummy",tenant="0000000-0000-0000-0000-000000000000",type="GeoUsedArchiveStorageMB"} 0
m365_sharepoint_usage_info{name="dummy",tenant="0000000-0000-0000-0000-000000000000",type="GeoUsedStorageMB"} 1.474813e+06
m365_sharepoint_usage_info{name="dummy",tenant="0000000-0000-0000-0000-000000000000",type="QuotaType"} 0
m365_sharepoint_usage_info{name="dummy",tenant="0000000-0000-0000-0000-000000000000",type="TenantStorageMB"} 5.357568e+06
```

## Useful queries
__This collector does not yet have any useful queries added, we would appreciate your help adding them!__

## Alerting examples
__This collector does not yet have alerting examples, we would appreciate your help adding them!__
