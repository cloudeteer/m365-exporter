# adsync collector

The adsync collector collects health metrics from Entra ID Connect Health.

## Configuration

None

## Metrics

| Name                                          | Description                                        | Type  | Labels                                  |
|-----------------------------------------------|----------------------------------------------------|-------|-----------------------------------------|
| `m365_adsync_on_premises_sync_enabled`        | status of Azure ad connect synchronization         | Gauge | `tenant`                                |
| `m365_adsync_on_premises_last_sync_date_time` | last Unix time of Azure ad connect synchronization | Gauge | `tenant`                                |
| `m365_adsync_on_premises_sync_error`          | count of Entra ID connect synchronization errors   | Gauge | `tenant`,`sync_service`, `error_bucket` |

## Example metric
__This collector does not yet have explained examples, we would appreciate your help adding them!__

## Useful queries
__This collector does not yet have any useful queries added, we would appreciate your help adding them!__

## Alerting examples
__This collector does not yet have alerting examples, we would appreciate your help adding them!__
