package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// The roundTripperFunc type is an adapter to allow the use of ordinary
// functions as RoundTrippers. If f is a function with the appropriate
// signature, RountTripperFunc(f) is a RoundTripper that calls f.
type roundTripperFunc func(req *http.Request) (*http.Response, error)

// RoundTrip implements the RoundTripper interface.
func (rt roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt(r)
}

type ctxHostValue struct{}

type HTTPClient struct {
	client *http.Client
}

func New(reg *prometheus.Registry) HTTPClient {
	histVec := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_client_request_duration_seconds",
			Help:    "Tracks the latencies for HTTP requests.",
			Buckets: []float64{0.1, 0.3, 0.6, 1, 3, 6, 9, 20},
		},
		[]string{"method", "host", "code"},
	)

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_client_requests_total",
			Help: "Tracks the number of HTTP requests.",
		}, []string{"method", "host", "code"},
	)

	inFlightGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_client_requests_inflight",
			Help: "Tracks the number of client requests currently in progress.",
		},
	)

	opts := promhttp.WithLabelFromCtx("host",
		func(ctx context.Context) string {
			if val, ok := ctx.Value(ctxHostValue{}).(string); ok {
				return val
			}

			return "unknown"
		},
	)

	reg.MustRegister(counter, histVec, inFlightGauge)

	hostRoundTripper := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		ctx := context.WithValue(req.Context(), ctxHostValue{}, req.Host)

		return http.DefaultTransport.RoundTrip(req.WithContext(ctx))
	})

	return HTTPClient{
		client: &http.Client{
			Transport: promhttp.InstrumentRoundTripperInFlight(inFlightGauge,
				promhttp.InstrumentRoundTripperCounter(counter,
					promhttp.InstrumentRoundTripperDuration(histVec,
						hostRoundTripper,
						opts),
					opts),
			),
		},
	}
}

func (c *HTTPClient) WithAzureCredential(cred azcore.TokenCredential) {
	transport := c.client.Transport
	c.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Host == "management.azure.com", req.Host == "outlook.office365.com":
			token, err := cred.GetToken(req.Context(), policy.TokenRequestOptions{
				Scopes: []string{fmt.Sprintf("https://%s/.default", req.Host)},
			})
			if err != nil {
				return nil, fmt.Errorf("getting token: %w", err)
			}

			req.Header.Set("Authorization", "Bearer "+token.Token)
		case strings.HasSuffix(req.Host, "-admin.sharepoint.com"):
			token, err := cred.GetToken(req.Context(), policy.TokenRequestOptions{
				Scopes: []string{fmt.Sprintf("https://%s/.default", req.Host)},
			})
			if err != nil {
				return nil, fmt.Errorf("getting token: %w", err)
			}

			req.Header.Set("Authorization", "Bearer "+token.Token)
		}

		return transport.RoundTrip(req)
	})
}

func (c *HTTPClient) GetHTTPClient() *http.Client {
	return c.client
}
