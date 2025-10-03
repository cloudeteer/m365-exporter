# application collector

The application collector collects metrics about Azure AD / Entra ID application registrations and their client secret expiration dates.

## Configuration

None

## Permissions Required

The service principal used by the exporter must have the following Microsoft Graph API permission:
- `Application.Read.All` (Application permission)

## Metrics

| Name                                                      | Description                                                          | Type  | Labels                                                    |
|-----------------------------------------------------------|----------------------------------------------------------------------|-------|-----------------------------------------------------------|
| `m365_application_client_secret_expiration_timestamp`     | The expiration timestamp of the client secret (Unix timestamp)       | Gauge | `appName`,`appID`,`secretName`,`keyID`,`tenant`           |
| `m365_application_client_secret_expired`                  | Whether the client secret has expired (1 = expired, 0 = valid)       | Gauge | `appName`,`appID`,`secretName`,`keyID`,`tenant`           |

### Labels Description

- `appName`: Display name of the application registration
- `appID`: Application (client) ID of the app registration
- `secretName`: Display name of the client secret (or truncated keyID if no name)
- `keyID`: Unique identifier (GUID) of the client secret
- `tenant`: Microsoft 365 tenant identifier

## Example metric

```
m365_application_client_secret_expiration_timestamp{appName="Production API",appID="12345678-1234-1234-1234-123456789abc",secretName="Prod Secret",keyID="abcd1234-5678-90ef-ghij-klmnopqrstuv",tenant="contoso.onmicrosoft.com"} 1.7409792e+09
m365_application_client_secret_expired{appName="Production API",appID="12345678-1234-1234-1234-123456789abc",secretName="Prod Secret",keyID="abcd1234-5678-90ef-ghij-klmnopqrstuv",tenant="contoso.onmicrosoft.com"} 0
```

## Useful queries

### Secrets expiring in the next 30 days
```promql
(m365_application_client_secret_expiration_timestamp - time()) < (30 * 24 * 3600)
and
m365_application_client_secret_expired == 0
```

### Secrets expiring in the next 7 days
```promql
(m365_application_client_secret_expiration_timestamp - time()) < (7 * 24 * 3600)
and
m365_application_client_secret_expired == 0
```

### Already expired secrets
```promql
m365_application_client_secret_expired == 1
```

### Days until expiration for all active secrets
```promql
(m365_application_client_secret_expiration_timestamp - time()) / (24 * 3600)
```

### Count of secrets by application
```promql
count by (appName, appID) (m365_application_client_secret_expiration_timestamp)
```

### Applications with expired secrets
```promql
count by (appName, appID) (m365_application_client_secret_expired == 1)
```

## Alerting examples

### Critical: Secret expired
```yaml
- alert: ClientSecretExpired
  expr: m365_application_client_secret_expired == 1
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "Client secret has expired for {{ $labels.appName }}"
    description: "The client secret '{{ $labels.secretName }}' for application '{{ $labels.appName }}' ({{ $labels.appID }}) has expired. Services using this secret will fail authentication."
```

### Warning: Secret expiring within 7 days
```yaml
- alert: ClientSecretExpiringSoon
  expr: |
    (m365_application_client_secret_expiration_timestamp - time()) < (7 * 24 * 3600)
    and
    m365_application_client_secret_expired == 0
  for: 1h
  labels:
    severity: warning
  annotations:
    summary: "Client secret expiring soon for {{ $labels.appName }}"
    description: "The client secret '{{ $labels.secretName }}' for application '{{ $labels.appName }}' ({{ $labels.appID }}) will expire in {{ $value | humanizeDuration }}. Please rotate the secret before expiration."
```

### Info: Secret expiring within 30 days
```yaml
- alert: ClientSecretExpiringIn30Days
  expr: |
    (m365_application_client_secret_expiration_timestamp - time()) < (30 * 24 * 3600)
    and
    (m365_application_client_secret_expiration_timestamp - time()) >= (7 * 24 * 3600)
    and
    m365_application_client_secret_expired == 0
  for: 6h
  labels:
    severity: info
  annotations:
    summary: "Client secret expiring in 30 days for {{ $labels.appName }}"
    description: "The client secret '{{ $labels.secretName }}' for application '{{ $labels.appName }}' ({{ $labels.appID }}) will expire in {{ $value | humanizeDuration }}. Plan to rotate this secret soon."
```

### Multiple expired secrets per application
```yaml
- alert: MultipleExpiredSecretsPerApp
  expr: |
    count by (appName, appID) (m365_application_client_secret_expired == 1) > 1
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "Multiple expired secrets for {{ $labels.appName }}"
    description: "Application '{{ $labels.appName }}' ({{ $labels.appID }}) has {{ $value }} expired client secrets. This may indicate a maintenance issue."
```
