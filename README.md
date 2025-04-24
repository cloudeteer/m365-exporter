<p align="center">
    <img src="docs/sources/assets/logo_m365_exporter.svg" alt="m365-exporter logo" height="120px">
</p>


<p align="center">
    <a href="https://github.com/cloudeteer/m365-exporter/actions?query=workflow%3ACI"><img src="https://github.com/cloudeteer/m365-exporter/workflows/CI/badge.svg" alt="CI"></a>
    <a href="https://github.com/cloudeteer/m365-exporter/blob/master/LICENSE.txt"><img src="https://img.shields.io/github/license/cloudeteer/m365-exporter" alt="GitHub license"></a>
    <a href="https://github.com/cloudeteer/m365-exporter/releases/latest"><img src="https://img.shields.io/github/release/cloudeteer/m365-exporter.svg?logo=github" alt="Current Release"></a>
    <a href="https://github.com/cloudeteer/m365-exporter/stargazers"><img src="https://img.shields.io/github/stars/cloudeteer/m365-exporter?style=flat&logo=github" alt="GitHub Repo stars"></a>
    <a href="https://github.com/cloudeteer/m365-exporter/releases/latest"><img src="https://img.shields.io/github/downloads/cloudeteer/m365-exporter/total?logo=github" alt="GitHub all releases"></a>
    <a href="https://goreportcard.com/report/github.com/cloudeteer/m365-exporter"><img src="https://goreportcard.com/badge/github.com/cloudeteer/m365-exporter" alt="Go Report Card"></a>
</p>


---

> [!NOTE]
> This repository is publicly accessible as part of our open-source initiative.
> We welcome contributions from the community alongside our organization's primary development efforts.

---

# m365-exporter

## About

A Microsoft 365 exporter for Prometheus metrics.
The exporter uses the Microsoft Graph API to collect metrics about Microsoft 365 services
and exports them in a format that can be scraped by Prometheus.

### Collectors

The following collectors are implemented:

| Name                                             | Description             |
|--------------------------------------------------|-------------------------|
| [adsync](docs/collector.adsync.md)               | Entra ID Connect Health |
| [exchange](docs/collector.exchange.md)           | Exchange Online metrics |
| [intune](docs/collector.intune.md)               | Intune devices          |
| [license](docs/collector.license.md)             | Licenses usage          |
| [onedrive](docs/collector.license.md)            | Onedrive usage          |
| [securescore](docs/collector.securescore.md)     | Securescore             |
| [servicehealth](docs/collector.servicehealth.md) | Service Health          |
| [teams](docs/collector.servicehealth.md)         | Teams                   |
| [sharepoint](docs/collector.sharepoint.md)       | Sharepoint              |

## Installation

### Single binary

Download the latest release from the [release page](https://github.com/cloudeteer/m365-exporter/releases/).

## Docker

A Docker image exists on [GitHub container registry](https://github.com/cloudeteer/m365-exporter/pkgs/container/m365-exporter).

Tags:

* `ghcr.io/cloudeteer/m365-exporter:<VERSION>`

## Configuration

### MS Graph Permissions

The exporter requires the following permissions to be set in the Entra ID app registration as Application permissions:

- DeviceManagementConfiguration.Read.All
- DeviceManagementManagedDevices.Read.All
- Directory.Read.All
- Files.Read.All
- Organization.Read.All
- SecurityEvents.Read.All
- ServiceHealth.Read.All
- Sites.Read.All
- TeamSettings.Read.All
- User.Read.All

Keep in mind, after granting the permissions, the administrator must consent to them.

### Exchange API Permissions

The exporter requires the following permissions to be set in the Entra ID app registration as Application permissions for the `Office 365 Exchange Online` App:

* Exchange.ManageAsApp

Also, the directory role `Secruity Reader` or `Global Reader` must be granted.

Ref: https://learn.microsoft.com/en-us/powershell/exchange/connect-exo-powershell-managed-identity?view=exchange-ps

### Entra ID Connect Health

Permissions for Entra ID Connect Health must be set in the on the Entra ID Connect Health page via permissions.
The `Read` permission is required.

### Sharepoint API Permissions

The exporter requires the following permissions to be set in the Entra ID app registration as Application permissions for the `Office 365 SharePoint Online` App:

* Sites.FullControl.All

### Via config file

By default, the exporter will search in `/etc/m365-exporter/` a file named `m365-exporter-config.yaml`, alternatively the file can be placed
in the current working directory of the program. It’s possible to set a specific location of the config file via setting
the `M365_CONFIGFILE` environment variable.

A fully fledged example config file can be found in the [docs folder](docs/m365-exporter-config.yaml).

| Config Parameter                          | Info                                                                                                 |
|-------------------------------------------|------------------------------------------------------------------------------------------------------|
| `settings.loglevel`                       | Possible values are "panic","fatal","error","warning","info","debug" and "trace". Default is "info". |
| `settings.serviceHealthStatusRefreshRate` | Refresh rate of service health status in minutes. Only Integers allowed. Default is 5 minutes.       |
| `settings.serviceHealthIssueKeepDays`     | Setting how long an Incident or Advisory should be kept as resolved in the metrics.                  |
| `onedrive.scrambleNames`                  | `bool` whether the label for individual onedrive metrics should have a scrambled version of the UPN  |
| `onedrive.scrambleSalt`                   | Set the salt to scramble the UPNs, a default value is set, so UPN hashes are always salted           |

Each collector can be disabled using this schema:

```yaml
oneDrive:
  enabled: false
teams:
  enabled: true
adsync:
  enabled: true
exchange:
  enabled: true
securescore:
  enabled: true
license:
  enabled: true
servicehealth:
  enabled: true
intune:
  enabled: true
entraid:
  enabled: true
sharepoint:
  enabled: true
```

### Via environment variables

Environment variables can be used to set configuration parameters. If a parameter is set via the environment, it takes precedence over
the settings in the config file.

The environment variable names correspond to a key in the YAML file,
by prefixing it with `M365_` and where the `.`s are replaced by `_`.
`M365_SERVER_HOST` set the `server.host` parameter in the YAML file.

> [!CAUTION]
> The config provider can’t handle boolean values passed through the environment.

### Authentication

m365-exporter supports all authentication supported by Azure SDK for Go.

#### Service principal with a secret

| Variable name         | Value                                        |
|-----------------------|----------------------------------------------|
| `AZURE_CLIENT_ID`     | Application ID of an Azure service principal |
| `AZURE_TENANT_ID`     | ID of the application's Azure AD tenant      |
| `AZURE_CLIENT_SECRET` | Password of the Azure service principal      |

#### Service principal with certificate

| Variable name                   | Value                                                                          |
|---------------------------------|--------------------------------------------------------------------------------|
| `AZURE_CLIENT_ID`               | Application ID of an Azure service principal                                   |
| `AZURE_TENANT_ID`               | ID of the application's Azure AD tenant                                        |
| `AZURE_CLIENT_CERTIFICATE_PATH` | Path to a certificate file including private key (without password protection) |

#### Use a managed identity

| Variable name     | Value                                                                              |
|-------------------|------------------------------------------------------------------------------------|
| `AZURE_CLIENT_ID` | User-assigned managed client ID. Can be avoid, if a system assign identity is used |
| `AZURE_TENANT_ID` | ID of the application's Azure AD tenant                                            |

## Supporting documentation

- [graph API reference](https://docs.microsoft.com/en-us/graph/api/overview?view=graph-rest-1.0) (includes required permissions)
- [pagination](https://docs.microsoft.com/en-us/graph/sdks/paging?tabs=Go)
- [Graph API Explorer](https://developer.microsoft.com/en-us/graph/graph-explorer) (look up API endpoints, and their required permissions)

## Building

Run `make help` to see available make targets.

To contribute to the project, refer to the [CONTRIBUTING.md](CONTRIBUTING.md) file.

## Commercial support

For commercial support, contact [Cloudeteer](https://www.cloudeteer.de/contact).

## License

This project is licensed under the [MIT License](LICENSE).
