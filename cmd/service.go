package cmd

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/runtime"
	"github.com/spf13/cobra"
)

var serviceCmd = &cobra.Command{
	Use:     "service",
	Aliases: []string{"srv", "serve", "standalone", "server"},
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger.Debug("creating promotion handler...")
		hdl, err := handler.NewPromotionHandler(
			handler.WithAuthMode(config.GitHub.AuthMode),
			handler.WithSSMKey(config.GitHub.SSMKey),
			handler.WithWebhookSecret(config.GitHub.WebhookSecret),
			handler.WithToken(os.Getenv("GITHUB_TOKEN")),
			handler.WithContext(cmd.Context()),
			handler.WithLogger(logger))
		if err != nil {
			return err
		}
		logger.Debug("creating runtime...")
		runtime := runtime.NewRuntime(hdl,
			runtime.WithLogger(logger.With("component", "runtime")))

		h := http.NewServeMux()
		h.HandleFunc(config.Service.Path, runtime.Service)

		s := &http.Server{
			Handler:      h,
			Addr:         net.JoinHostPort(config.Service.Addr, config.Service.Port),
			WriteTimeout: config.Service.Timeout,
			ReadTimeout:  config.Service.Timeout,
			IdleTimeout:  config.Service.Timeout,
		}

		logger.Info("service starting...",
			slog.String("service", fmt.Sprintf("%+v", config.Service)),
			slog.String("authMode", config.GitHub.AuthMode))
		return s.ListenAndServe()
	},
}

func init() {
	bindEnvMap(serviceCmd, svcEnvMapString)
	bindEnvMap(serviceCmd, svcEnvMapDuration)
}
