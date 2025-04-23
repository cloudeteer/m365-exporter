package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/cloudeteer/m365-exporter/pkg/auth"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/abstract"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/adsync"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/entraid"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/exchange"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/intune"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/license"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/onedrive"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/securescore"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/servicehealth"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/sharepoint"
	"github.com/cloudeteer/m365-exporter/pkg/collectors/teams"
	"github.com/cloudeteer/m365-exporter/pkg/conf"
	"github.com/cloudeteer/m365-exporter/pkg/health"
	"github.com/cloudeteer/m365-exporter/pkg/httpclient"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	v "github.com/spf13/viper"
)

const metricsEndpoint = "/metrics"

// main is the entry point of the app.
// It an wrapper around run function to handle the exit code.
// It exits with the exit code returned by run.
func main() {
	os.Exit(run(os.Stdout))
}

// run starts the server and returns the exit code.
func run(logWriter io.Writer) int {
	ctx := context.Background()

	var logLevel slog.LevelVar

	logLevel.Set(slog.LevelInfo)

	logger := slog.New(slog.NewJSONHandler(logWriter, &slog.HandlerOptions{
		Level: &logLevel,
	}))

	err := conf.Configure(logger)
	if err != nil {
		logger.ErrorContext(ctx, "error while configuring exporter", slog.Any("err", err))

		return 1
	}

	err = logLevel.UnmarshalText([]byte(v.GetString(conf.KeyLogLevel)))
	if err != nil {
		logger.ErrorContext(ctx, "unable to set log level", slog.Any("err", err))

		return 1
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	reg := prometheus.NewRegistry()

	httpClient := httpclient.New(reg)

	msGraphClient, azureCredential, err := auth.NewMSGraphClient(httpClient.GetHTTPClient())
	if err != nil {
		logger.ErrorContext(ctx, "failed to authenticate against Microsoft",
			slog.Any("error", err),
		)

		return 1
	}

	httpClient.WithAzureCredential(azureCredential)

	// register default collectors from github.com/prometheus/client_golang/prometheus/collectors
	reg.MustRegister(version.NewCollector("m365_exporter"))
	reg.MustRegister(collectors.NewBuildInfoCollector())
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	tenantID := v.GetString(conf.KeyAzureTenantID)

	if err := setupMetricsCollectors(ctx, logger, reg, tenantID, msGraphClient, httpClient.GetHTTPClient()); err != nil {
		logger.ErrorContext(ctx, "failed to setup metrics collectors",
			slog.Any("error", err),
		)

		return 1
	}

	stdErrorLog := slog.NewLogLogger(logger.Handler(), slog.LevelError)

	promHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorLog: stdErrorLog,
	})

	listenAddr := fmt.Sprintf("%s:%s", v.GetString(conf.KeySrvHost), v.GetString(conf.KeySrvPort))

	http.Handle(metricsEndpoint, promHandler)
	http.Handle("/health", health.NewHandler(logger, listenAddr, metricsEndpoint))

	server := &http.Server{
		Addr:              listenAddr,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      4 * time.Minute,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          stdErrorLog,
	}

	logger.InfoContext(ctx, "listening on "+listenAddr)

	errCh := make(chan error, 1)

	// start server in a goroutine to be able to gracefully shutdown the server
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}

		close(errCh)
	}()

	select {
	case <-ctx.Done():
		// shutdown server gracefully if [Ctrl]+[C] was pressed
		logger.InfoContext(ctx, "shutting down server")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			logger.ErrorContext(ctx, "failed to shutdown server",
				slog.Any("error", err),
			)
		}
	case err := <-errCh:
		logger.ErrorContext(ctx, "server encountered an error",
			slog.Any("error", err),
		)

		return 1
	}

	return 0
}

func setupMetricsCollectors(
	ctx context.Context, logger *slog.Logger,
	reg *prometheus.Registry, tenantID string,
	msGraphClient *msgraphsdk.GraphServiceClient, httpClient *http.Client,
) error {
	for _, val := range []struct {
		collector abstract.Collector
		interval  time.Duration
		enabled   bool
	}{
		{
			collector: adsync.NewCollector(logger, tenantID, msGraphClient, httpClient),
			interval:  1 * time.Hour,
			enabled:   v.GetBool(conf.KeyAdsSyncEnabled),
		},
		{
			collector: exchange.NewCollector(logger, tenantID, httpClient),
			interval:  1 * time.Hour,
			enabled:   v.GetBool(conf.KeyExchangeEnabled),
		},
		{
			collector: securescore.NewCollector(logger, tenantID, msGraphClient),
			interval:  1 * time.Hour,
			enabled:   v.GetBool(conf.KeySecureScoreEnabled),
		},
		{
			collector: license.NewCollector(logger, tenantID, msGraphClient),
			interval:  1 * time.Hour,
			enabled:   v.GetBool(conf.KeyLicenseEnabled),
		},
		{
			collector: servicehealth.NewCollector(logger, tenantID, msGraphClient),
			interval:  time.Duration(v.GetInt(conf.KeyServiceHealthStatusRefreshRate)) * time.Minute,
			enabled:   v.GetBool(conf.KeyServiceHealthEnabled),
		},
		{
			collector: intune.NewCollector(logger, tenantID, msGraphClient),
			interval:  3 * time.Hour,
			enabled:   v.GetBool(conf.KeyIntuneEnabled),
		},
		{
			collector: onedrive.NewCollector(logger, tenantID, msGraphClient, onedrive.Settings{
				ScrambleSalt:  v.GetString(conf.KeyODriveScrambleSalt),
				ScrambleNames: v.GetBool(conf.KeyODriveScrambleNames),
			}),
			interval: 3 * time.Hour,
			enabled:  v.GetBool(conf.KeyODriveEnabled),
		},
		{
			collector: teams.NewCollector(logger, tenantID, msGraphClient),
			interval:  3 * time.Hour,
			enabled:   v.GetBool(conf.KeyTeamsEnabled),
		},
		{
			collector: entraid.NewCollector(logger, tenantID, msGraphClient),
			interval:  3 * time.Hour,
			enabled:   v.GetBool(conf.KeyEntraIDEnabled),
		},
		{
			collector: sharepoint.NewCollector(logger, tenantID, msGraphClient, httpClient),
			interval:  1 * time.Hour,
			enabled:   v.GetBool(conf.KeySharePointEnabled),
		},
	} {
		if !val.enabled {
			logger.InfoContext(ctx, "collector disabled, skipping registration",
				slog.String("collector", val.collector.(interface{ GetSubsystem() string }).GetSubsystem()))
			continue
		}

		if err := reg.Register(val.collector); err != nil {
			return fmt.Errorf("failed to register collector: %w", err)
		}

		val.collector.StartBackgroundWorker(ctx, val.interval)
	}

	return nil
}
