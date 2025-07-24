package health

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
)

func NewHandler(logger *slog.Logger, listenAddr, metricsEndpoint string) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, req *http.Request) {
		// required to remedy 10k issues for liveness probes by kubelet
		// -> cuts off the payload but checks if metrics can be received
		var target string

		if strings.HasPrefix(listenAddr, ":") || strings.HasPrefix(listenAddr, "0.0.0.0") {
			target = "http://localhost" + listenAddr
		} else {
			target = "http://" + listenAddr
		}

		target += metricsEndpoint

		//nolint: noctx
		resp, err := http.DefaultClient.Head(target)
		if err != nil {
			logger.ErrorContext(req.Context(), "health endpoint encountered an error while querying metrics",
				slog.Any("err", err),
			)

			responseWriter.WriteHeader(http.StatusInternalServerError)

			return
		}

		_, err = io.ReadAll(resp.Body)
		if err != nil {
			responseWriter.WriteHeader(http.StatusInternalServerError)

			return
		}

		_ = resp.Body.Close()

		responseWriter.WriteHeader(resp.StatusCode)
	})
}
