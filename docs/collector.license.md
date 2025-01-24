# license collector

The license collector collects metrics about license and license group assignments.

## Configuration

None

## Metrics

| Name                        | Description                     | Type  | Labels                                     |
|-----------------------------|---------------------------------|-------|--------------------------------------------|
| `m365_license_current`      | current amount of licenses      | Gauge | `license`,`tenant`                         |
| `m365_license_total`        | total amount of licenses        | Gauge | `license`,`tenant`,`status`                |
| `m365_license_status`       | status of licenses              | Gauge | `license`,`tenant`                         |
| `m365_license_group_errors` | group license assignment errors | Gauge | `license`,`tenant`,`group_name`,`group_id` |

## Example metric
__This collector does not yet have explained examples, we would appreciate your help adding them!__

## Useful queries
__This collector does not yet have any useful queries added, we would appreciate your help adding them!__

## Alerting examples
__This collector does not yet have alerting examples, we would appreciate your help adding them!__
