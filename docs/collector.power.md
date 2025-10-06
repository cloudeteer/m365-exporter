# power collector

The power collector collects capacity and licensing metrics from Microsoft Power Platform.

## Configuration

None

## Metrics

| Name | Description | Type | Labels |
|------|-------------|------|--------|
| `m365_power_capacity_rated_consumption` | Rated consumption of M365 Power Capacity in MB | Gauge | `tenant`, `capacityType`, `capacityUnits` |
| `m365_power_capacity_entitlement_total` | Total capacity provided by a specific entitlement in MB | Gauge | `tenant`, `capacityType`, `capacitySubType` |
| `m365_power_capacity_licenses_paid_status_count` | The number of paid licenses by status | Gauge | `tenant`, `entitlementCode`, `displayName`, `skuId`, `status`, `capacityType`, `capacitySubType` |
| `m365_power_capacity_licenses_trial_status_count` | The number of trial licenses by status | Gauge | `tenant`, `entitlementCode`, `displayName`, `skuId`, `status`, `capacityType`, `capacitySubType` |

## Example metrics

```prometheus
# HELP m365_power_capacity_rated_consumption Rated consumption of M365 Power Capacity in MB
# TYPE m365_power_capacity_rated_consumption gauge
m365_power_capacity_rated_consumption{capacityType="ApiCallCount",capacityUnits="None",tenant="0000000-0000-0000-0000-000000000000"} 0
m365_power_capacity_rated_consumption{capacityType="CapacityPass",capacityUnits="Unit",tenant="0000000-0000-0000-0000-000000000000"} 0
m365_power_capacity_rated_consumption{capacityType="Database",capacityUnits="MB",tenant="0000000-0000-0000-0000-000000000000"} 41424.554000000004
m365_power_capacity_rated_consumption{capacityType="File",capacityUnits="MB",tenant="0000000-0000-0000-0000-000000000000"} 31126.955

# HELP m365_power_capacity_entitlement_total Total capacity provided by a specific entitlement in MB
# TYPE m365_power_capacity_entitlement_total gauge
m365_power_capacity_entitlement_total{capacitySubType="ApiCallCountBase",capacityType="ApiCallCount",tenant="0000000-0000-0000-0000-000000000000"} 4e+07
m365_power_capacity_entitlement_total{capacitySubType="ApiCallCountIncremental",capacityType="ApiCallCount",tenant="0000000-0000-0000-0000-000000000000"} 520000
m365_power_capacity_entitlement_total{capacitySubType="DatabaseBase",capacityType="Database",tenant="0000000-0000-0000-0000-000000000000"} 10240
m365_power_capacity_entitlement_total{capacitySubType="DatabaseIncremental",capacityType="Database",tenant="0000000-0000-0000-0000-000000000000"} 41910

# HELP m365_power_capacity_licenses_paid_status_count The number of paid licenses by status
# TYPE m365_power_capacity_licenses_paid_status_count gauge
m365_power_capacity_licenses_paid_status_count{capacitySubType="DatabaseBase",capacityType="Database",displayName="MS Project",entitlementCode="D365_cds_Project",skuId="0000000-0000-0000-0000-000000000000",status="enabled",tenant="0000000-0000-0000-0000-000000000000"} 40
m365_power_capacity_licenses_paid_status_count{capacitySubType="DatabaseBase",capacityType="Database",displayName="MS Project",entitlementCode="D365_cds_Project",skuId="0000000-0000-0000-0000-000000000000",status="warning",tenant="0000000-0000-0000-0000-000000000000"} 0
m365_power_capacity_licenses_paid_status_count{capacitySubType="DatabaseBase",capacityType="Database",displayName="MS Project",entitlementCode="D365_cds_Project",skuId="0000000-0000-0000-0000-000000000000",status="suspended",tenant="0000000-0000-0000-0000-000000000000"} 0

# HELP m365_power_capacity_licenses_trial_status_count The number of trial licenses by status
# TYPE m365_power_capacity_licenses_trial_status_count gauge
m365_power_capacity_licenses_trial_status_count{capacitySubType="ApiCallCountBase",capacityType="ApiCallCount",displayName="",entitlementCode="D365_cds_Dev_Viral",skuId="0000000-0000-0000-0000-000000000000",status="enabled",tenant="0000000-0000-0000-0000-000000000000"} 0
m365_power_capacity_licenses_trial_status_count{capacitySubType="ApiCallCountBase",capacityType="ApiCallCount",displayName="",entitlementCode="D365_cds_Dev_Viral",skuId="0000000-0000-0000-0000-000000000000",status="suspended",tenant="0000000-0000-0000-0000-000000000000"} 0
```

## Useful queries
__This collector does not yet have any useful queries added, we would appreciate your help adding them!__

## Alerting examples
__This collector does not yet have alerting examples, we would appreciate your help adding them!__
