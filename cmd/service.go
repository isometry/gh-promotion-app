package cmd

import (
	"net"
	"net/http"
	"time"

	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/runtime"
	"github.com/spf13/cobra"
)

var (
	svcHostPath, svcHostAddr, svcHostPort string
	svcIoTimeout                          time.Duration
)

var serviceCmd = &cobra.Command{
	Use:     "service",
	Aliases: []string{"s", "serve", "standalone", "server"},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		loadViperVariables(cmd)
		logger = logger.With("mode", "service")
		logger.Info("spawning...")

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		logger.Debug("creating promotion handler...")
		hdl, err := handler.NewPromotionHandler(
			handler.WithWebhookSecret(webhookSecret),
			handler.WithAuthMode(githubAuthMode),
			handler.WithToken(githubToken),
			handler.WithSSMKey(githubSSMKey),
			handler.WithContext(cmd.Context()),
			handler.WithLogger(logger.With("component", "promotion-handler")))
		if err != nil {
			return err
		}

		logger.Debug("creating runtime...")
		runtime := runtime.NewRuntime(hdl,
			runtime.WithLogger(logger.With("component", "runtime")))

		logger.Debug("creating HTTP server...")
		h := http.NewServeMux()
		h.HandleFunc(svcHostPath, runtime.ServeHTTP)

		s := &http.Server{
			Handler:      h,
			Addr:         net.JoinHostPort(svcHostAddr, svcHostPort),
			WriteTimeout: svcIoTimeout,
			ReadTimeout:  svcIoTimeout,
			IdleTimeout:  svcIoTimeout,
		}

		logger.Info("serving...", "address", s.Addr, "path", svcHostPath, "timeout", svcIoTimeout.String())
		return s.ListenAndServe()
	},
}

func init() {
	bindEnvMap(serviceCmd, svcEnvMapString)
	bindEnvMap(serviceCmd, svcEnvMapDuration)
}
