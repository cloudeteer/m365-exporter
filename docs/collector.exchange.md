# exchange collector

The exchange collector collects exchange online metrics from Outlook Admin BETA REST API (`https://outlook.office365.com/adminapi/beta/<tenant-id>/InvokeCommand`)

## Configuration

None

## Metrics

| Name                              | Description                                | Type  | Labels                                            |
|-----------------------------------|--------------------------------------------|-------|---------------------------------------------------|
| `m365_exchange_mailflow_messages` | status of azure ad connect synchronization | Gauge | `tenant`, `organization`,`direction`,`event_type` |

## Example metric

```
m365_exchange_mailflow_messages{direction="Inbound",event_type="EdgeBlockSpam",organization="contoso.onmicrosoft.com",tenant="00000000-0000-0000-0000-000000000000"} 542
m365_exchange_mailflow_messages{direction="Inbound",event_type="EmailPhish",organization="contoso.onmicrosoft.com",tenant="00000000-0000-0000-0000-000000000000"} 56
m365_exchange_mailflow_messages{direction="Inbound",event_type="GoodMail",organization="contoso.onmicrosoft.com",tenant="00000000-0000-0000-0000-000000000000"} 16443
m365_exchange_mailflow_messages{direction="Inbound",event_type="SpamDetections",organization="contoso.onmicrosoft.com",tenant="00000000-0000-0000-0000-000000000000"} 558
m365_exchange_mailflow_messages{direction="IntraOrg",event_type="GoodMail",organization="contoso.onmicrosoft.com",tenant="00000000-0000-0000-0000-000000000000"} 19311
m365_exchange_mailflow_messages{direction="Outbound",event_type="GoodMail",organization="contoso.onmicrosoft.com",tenant="00000000-0000-0000-0000-000000000000"} 2671
```

## Useful queries
__This collector does not yet have any useful queries added, we would appreciate your help adding them!__

## Alerting examples
__This collector does not yet have alerting examples, we would appreciate your help adding them!__
