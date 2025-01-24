# onedrive collector

The onedrive collector collects metrics about sharepoint libraries and personal onedrive usage.

## Configuration

| Config Parameter         | Info                                                                                                |
|--------------------------|-----------------------------------------------------------------------------------------------------|
| `onedrive.scrambleNames` | `bool` whether the label for individual onedrive metrics should have a scrambled version of the UPN |
| `onedrive.scrambleSalt`  | Set the salt to scramble the UPNs, a default value is set, so UPN hashes are always salted          |

## Metrics

| Name                                  | Description                                           | Type  | Labels                                 |
|---------------------------------------|-------------------------------------------------------|-------|----------------------------------------|
| `m365_onedrive_total_available_bytes` | the total amount of available bytes for this onedrive | Gauge | `owner`,`driveType`,`driveID`,`tenant` |
| `m365_onedrive_used_bytes`            | number of bytes used on this onedrive                 | Gauge | `owner`,`driveType`,`driveID`,`tenant` |
| `m365_onedrive_deleted_bytes`         | number of bytes in recycle bin                        | Gauge | `owner`,`driveType`,`driveID`,`tenant` |

## Example metric
__This collector does not yet have explained examples, we would appreciate your help adding them!__

## Useful queries
__This collector does not yet have any useful queries added, we would appreciate your help adding them!__

## Alerting examples
__This collector does not yet have alerting examples, we would appreciate your help adding them!__
