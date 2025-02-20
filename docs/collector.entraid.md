# entraid collector

The entraid collector collects information about entra ID users.

## Configuration

None

## Metrics

| Name                           | Description                              | Type  | Labels                      |
|--------------------------------|------------------------------------------|-------|-----------------------------|
| `m365_entraid_user_count`      | Amount of users with specific attributes | Gauge | `tenant`, `type`, `enabled` |

## Example metric

```
# HELP m365_entraid_user_count User metrics in Entra ID
# TYPE m365_entraid_user_count gauge
m365_entraid_user_count{enabled="false",tenant="0000000-0000-0000-0000-000000000000",type="Guest"} 0
m365_entraid_user_count{enabled="false",tenant="0000000-0000-0000-0000-000000000000",type="Member"} 264
m365_entraid_user_count{enabled="true",tenant="0000000-0000-0000-0000-000000000000",type="Guest"} 2097
m365_entraid_user_count{enabled="true",tenant="0000000-0000-0000-0000-000000000000",type="Member"} 2072
```

## Useful queries
__This collector does not yet have any useful queries added, we would appreciate your help adding them!__

## Alerting examples
__This collector does not yet have alerting examples, we would appreciate your help adding them!__
