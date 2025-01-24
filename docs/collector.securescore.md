# securescore collector

The securescore collector collects metrics Tenant secure score.

The compliance score is a different score and not available through MS Graph API.

## Configuration

None

## Metrics

| Name                       | Description                         | Type  | Labels   |
|----------------------------|-------------------------------------|-------|----------|
| `m365_securescore_current` | Currently achieved secure score     | Gauge | `tenant` |
| `m365_securescore_max`     | The maximum achievable secure score | Gauge | `tenant` |

## Example metric
__This collector does not yet have explained examples, we would appreciate your help adding them!__

## Useful queries
__This collector does not yet have any useful queries added, we would appreciate your help adding them!__

## Alerting examples
__This collector does not yet have alerting examples, we would appreciate your help adding them!__
