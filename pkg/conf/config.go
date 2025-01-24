package conf

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	v "github.com/spf13/viper"
)

const (
	envPrefix = "M365"

	KeyCfgFile = "configfile"

	KeySrvHost = "server.host"
	KeySrvPort = "server.port"

	KeyLogLevel                       = "settings.loglevel"
	KeyServiceHealthStatusRefreshRate = "settings.serviceHealthStatusRefreshRate"
	KeyserviceHealthIssueKeepDays     = "settings.serviceHealthIssueKeepDays"
	KeyAzureTenantID                  = "azure.tenantId"

	KeyODriveScrambleNames = "onedrive.scrambleNames"
	KeyODriveScrambleSalt  = "onedrive.scrambleSalt"
	KeyODriveEnabled       = "onedrive.enabled"

	KeyTeamsEnabled = "teams.enabled"
)

// required in order to avoid global var.
func getConfigLocations() []string {
	return []string{
		"/etc/m365-exporter/",
		"./",
	}
}

func Configure(logger *slog.Logger) error {
	v.SetDefault(KeySrvHost, "")
	v.SetDefault(KeySrvPort, "8080")
	v.SetDefault(KeyLogLevel, "info")
	v.SetDefault(KeyServiceHealthStatusRefreshRate, 5)
	v.SetDefault(KeyserviceHealthIssueKeepDays, 30)

	v.SetDefault(KeyODriveScrambleNames, true)
	v.SetDefault(KeyODriveScrambleSalt, "NsVfe9cRaH")
	v.SetDefault(KeyODriveEnabled, true)

	v.SetDefault(KeyTeamsEnabled, true)

	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.BindEnv(KeyAzureTenantID, "AZURE_TENANT_ID"); err != nil {
		return fmt.Errorf("could not bind environment variable AZURE_TENANT_ID: %w", err)
	}

	v.SetConfigName("m365-exporter-config")
	v.SetConfigType("yaml")

	if cfg := v.GetString(KeyCfgFile); cfg != "" {
		// search config in the porvided location only
		v.SetConfigFile(cfg)
	}

	for _, path := range getConfigLocations() {
		v.AddConfigPath(path)
	}

	configErr := v.ReadInConfig()
	if configErr != nil {
		var configNotfoundError v.ConfigFileNotFoundError
		if !errors.As(configErr, &configNotfoundError) {
			return fmt.Errorf("encountered a fatal error while reading the configuration in file %s: %w", v.ConfigFileUsed(), configErr)
		}

		logger.Info(fmt.Sprintf("did not find a config file in any of %s, using defaults and environment", getConfigLocations()))
	}

	// set Azure env for Azure SDK
	if err := os.Setenv("AZURE_TENANT_ID", v.GetString(KeyAzureTenantID)); err != nil {
		return fmt.Errorf("could not set environment variable AZURE_TENANT_ID: %w", err)
	}

	// check for mandatory fields
	if !v.IsSet(KeyAzureTenantID) {
		return fmt.Errorf("missing mandatory config parameter for %s", KeyAzureTenantID)
	}

	// check if service health status refresh rate is an int
	if _, err := strconv.ParseInt(v.GetString(KeyServiceHealthStatusRefreshRate), 10, 64); err != nil {
		logger.Warn("ServiceHealthStatusRefreshRate is no integer. Setting it to default which is 5 minutes", slog.Any("err", err))
		v.Set(KeyServiceHealthStatusRefreshRate, 5)
	}

	return nil
}
