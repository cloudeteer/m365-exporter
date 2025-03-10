# intune collector

The intune collector collects metrics and status about managed devices.

## Configuration

None

## Metrics

| Name                            | Description                                               | Type  | Labels                            |
|---------------------------------|-----------------------------------------------------------|-------|-----------------------------------|
| `m365_intune_device_compliance` | Compliance of devices managed by Intune                   | Gauge | `tenant`, `type`                  |
| `m365_intune_device_count`      | Device information of devices managed by Intune           | Gauge | `tenant`, `os_name`, `os_version` |

## Example metric

```
# HELP m365_intune_device_compliance Compliance of devices managed by Intune
# TYPE m365_intune_device_compliance gauge
m365_intune_device_compliance{tenant="0000000-0000-0000-0000-000000000000",type="all"} 2226
m365_intune_device_compliance{tenant="0000000-0000-0000-0000-000000000000",type="compliant"} 582
m365_intune_device_compliance{tenant="0000000-0000-0000-0000-000000000000",type="conflict"} 0
m365_intune_device_compliance{tenant="0000000-0000-0000-0000-000000000000",type="error"} 0
m365_intune_device_compliance{tenant="0000000-0000-0000-0000-000000000000",type="graceperiod"} 0
m365_intune_device_compliance{tenant="0000000-0000-0000-0000-000000000000",type="noncompliant"} 325
m365_intune_device_compliance{tenant="0000000-0000-0000-0000-000000000000",type="notapplicable"} 0
m365_intune_device_compliance{tenant="0000000-0000-0000-0000-000000000000",type="remediated"} 0
m365_intune_device_compliance{tenant="0000000-0000-0000-0000-000000000000",type="unknown"} 2
# HELP m365_intune_device_count Device information of devices managed by Intune
# TYPE m365_intune_device_count gauge
m365_intune_device_count{os_name="Android",os_version="10.0",tenant="0000000-0000-0000-0000-000000000000"} 7
m365_intune_device_count{os_name="Android",os_version="11",tenant="0000000-0000-0000-0000-000000000000"} 1
m365_intune_device_count{os_name="Android",os_version="11.0",tenant="0000000-0000-0000-0000-000000000000"} 8
m365_intune_device_count{os_name="Android",os_version="12",tenant="0000000-0000-0000-0000-000000000000"} 7
m365_intune_device_count{os_name="Windows",os_version="10.0.19045.5247",tenant="0000000-0000-0000-0000-000000000000"} 49
m365_intune_device_count{os_name="Windows",os_version="10.0.19045.5371",tenant="0000000-0000-0000-0000-000000000000"} 817
m365_intune_device_count{os_name="Windows",os_version="10.0.19045.5487",tenant="0000000-0000-0000-0000-000000000000"} 7
m365_intune_device_count{os_name="Windows",os_version="10.0.22000.978",tenant="0000000-0000-0000-0000-000000000000"} 1
m365_intune_device_count{os_name="Windows",os_version="10.0.22621.525",tenant="0000000-0000-0000-0000-000000000000"} 1
m365_intune_device_count{os_name="Windows",os_version="10.0.22631.3296",tenant="0000000-0000-0000-0000-000000000000"} 1
m365_intune_device_count{os_name="iOS",os_version="17.5",tenant="0000000-0000-0000-0000-000000000000"} 1
m365_intune_device_count{os_name="iOS",os_version="17.5.1",tenant="0000000-0000-0000-0000-000000000000"} 3
m365_intune_device_count{os_name="iOS",os_version="17.6.1",tenant="0000000-0000-0000-0000-000000000000"} 4
m365_intune_device_count{os_name="iOS",os_version="17.7",tenant="0000000-0000-0000-0000-000000000000"} 1
m365_intune_device_count{os_name="iOS",os_version="18.0",tenant="0000000-0000-0000-0000-000000000000"} 1
```

## Useful queries
__This collector does not yet have any useful queries added, we would appreciate your help adding them!__

## Alerting examples
__This collector does not yet have alerting examples, we would appreciate your help adding them!__
