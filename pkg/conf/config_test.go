package conf_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/cloudeteer/m365-exporter/pkg/conf"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ConfigReading(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	envTenantID := "AZURE_TENANT_ID"
	t.Setenv(envTenantID, "dummy")

	envCfg := "M365_CONFIGFILE"
	t.Setenv(envCfg, "./testdata/empty.yaml")

	err := conf.Configure(logger)
	require.NoError(t, err)

	t.Run("Test default values", func(t *testing.T) {
		assert.Equal(t, "8080", viper.Get(conf.KeySrvPort))
		assert.Equal(t, "", viper.GetString(conf.KeySrvHost))
	})

	t.Setenv(envCfg, "./testdata/listen.yaml")

	err = conf.Configure(logger)
	require.NoError(t, err)
	t.Run("Test reading yaml", func(t *testing.T) {
		assert.Equal(t, 8081, viper.Get(conf.KeySrvPort))
	})

	envPort := "M365_SERVER_PORT"
	t.Setenv(envPort, "8081")
	t.Run("Test env override", func(t *testing.T) {
		assert.Equal(t, "8081", viper.Get(conf.KeySrvPort))
	})
}

func TestHuntingDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	t.Setenv("AZURE_TENANT_ID", "dummy")
	t.Setenv("M365_CONFIGFILE", "./testdata/listen.yaml")
	t.Setenv("M365_HUNTING_ENABLED", "true")

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	err := conf.Configure(logger)
	require.NoError(t, err)
	assert.True(t, viper.GetBool(conf.KeyHuntingEnabled))
	assert.Equal(t, "1h", viper.GetString(conf.KeyHuntingInterval))
}
