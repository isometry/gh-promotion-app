package cmd

import (
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/isometry/gh-promotion-app/internal/runtime"
	"github.com/spf13/cobra"
	"net"
	"net/http"
	"time"
)

var (
	svcHostPath, svcHostAddr, svcHostPort string
	svcIoTimeout                          time.Duration
)

var serviceCmd = &cobra.Command{
	Use:     "service",
	Aliases: []string{"s", "serve", "standalone", "server"},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		logger = logger.With("mode", "service")
		logger.Info("Spawning...")

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var promoter *promotion.Promoter
		if !dynamicPromoter {
			promoter = promotion.NewDefaultPromoter()
		} else {
			logger.Info("Dynamic promoter activated...")
		}
		logger.Debug("Creating promotion handler...")
		hdl, err := handler.NewPromotionHandler(
			handler.WithWebhookSecret(webhookSecret),
			handler.WithAuthMode(githubAuthMode),
			handler.WithToken(githubToken),
			handler.WithSSMKey(githubSSMKey),
			handler.WithPromoter(promoter),
			handler.WithDynamicPromoterKey(dynamicPromoterKey),
			handler.WithContext(cmd.Context()),
			handler.WithLogger(logger.With("component", "promotion-handler")))
		if err != nil {
			return err
		}

		logger.Debug("Creating runtime...")
		runtime := runtime.NewRuntime(hdl,
			runtime.WithLogger(logger.With("component", "runtime")))

		logger.Debug("Creating HTTP server...")
		h := http.NewServeMux()
		h.HandleFunc(svcHostPath, runtime.ServeHTTP)

		s := &http.Server{
			Handler:      h,
			Addr:         net.JoinHostPort(svcHostAddr, svcHostPort),
			WriteTimeout: svcIoTimeout,
			ReadTimeout:  svcIoTimeout,
			IdleTimeout:  svcIoTimeout,
		}

		logger.Info("Serving...", "address", s.Addr, "path", svcHostPath, "timeout", svcIoTimeout.String())
		return s.ListenAndServe()
	},
}

func init() {
	bindEnvMap(serviceCmd, svcEnvMapString)
	bindEnvMap(serviceCmd, svcEnvMapDuration)
}
