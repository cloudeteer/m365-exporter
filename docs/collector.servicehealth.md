# servicehealth collector

The servicehealth collector collects metrics Tenant secure score.

## Configuration

| Config Parameter                          | Info                                                                                                 |
|-------------------------------------------|------------------------------------------------------------------------------------------------------|
| `settings.serviceHealthStatusRefreshRate` | Refresh rate of service health status in minutes. Only Integers allowed. Default is 5 minutes.       |
| `settings.serviceHealthIssueKeepDays`     | Setting how long an Incident or Advisory should be kept as resolved in the metrics.                  |

## Metrics

| Name                        | Description                                                                                               | Type  | Labels                                                                                                       |
|-----------------------------|-----------------------------------------------------------------------------------------------------------|-------|--------------------------------------------------------------------------------------------------------------|
| `m365_service_health`       | Represents the health status of a service. For the status mapping see the m365_service_health_info metric | Gauge | `service_name`,`service_id`,`tenant`                                                                         |
| `m365_service_health_info`  | companion metric for the service health metric. It is used to map the status to a number.                 | Gauge | `tenant`,`use`                                                                                               |
| `m365_service_health_issue` | health issue of a specific service                                                                        | Gauge | `tenant`,`service_name`,`classification`,`issue_create_timestamp`,`title`,`issue_id`,`issue_close_timestamp` |

## Example metric
__This collector does not yet have explained examples, we would appreciate your help adding them!__

## Useful queries
__This collector does not yet have any useful queries added, we would appreciate your help adding them!__

## Alerting examples
__This collector does not yet have alerting examples, we would appreciate your help adding them!__
