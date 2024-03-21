package cmd

import (
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/runtime"
	"github.com/spf13/cobra"
	"net"
	"net/http"
	"time"
)

var (
	hostPath, hostAddr, hostPort string
	ioTimeout                    time.Duration
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
		logger.Debug("Creating promotion handler...")
		hdl, err := handler.NewPromotionHandler(
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
		h.HandleFunc(hostPath, runtime.ServeHTTP)

		s := &http.Server{
			Handler:      h,
			Addr:         net.JoinHostPort(hostAddr, hostPort),
			WriteTimeout: ioTimeout,
			ReadTimeout:  ioTimeout,
			IdleTimeout:  ioTimeout,
		}

		logger.Info("Serving...", "address", s.Addr, "path", hostPath, "timeout", ioTimeout.String())
		return s.ListenAndServe()
	},
}

func init() {
	serviceCmd.PersistentFlags().StringVarP(&hostPath, "path", "P", "/", "host path")
	serviceCmd.PersistentFlags().StringVarP(&hostAddr, "host", "H", "", "host address. If not specified, listens on all interfaces in dual-stack mode when available.")
	serviceCmd.PersistentFlags().StringVarP(&hostPort, "port", "p", "8443", "host port")
	serviceCmd.PersistentFlags().DurationVarP(&ioTimeout, "timeout", "t", 1*time.Second, "I/O timeout")
}
