package cmd

import (
	"github.com/isometry/gh-promotion-app/cmd/helpers"
	"github.com/isometry/gh-promotion-app/internal/handler"
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
	PostRunE: func(cmd *cobra.Command, args []string) error {
		logger = logger.With("mode", "service")
		logger.Info("spawning...")

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		hdl, err := handler.NewPromotionHandler()
		if err != nil {
			return err
		}

		runtime := helpers.Runtime{Handler: hdl}

		h := http.NewServeMux()
		h.HandleFunc(hostPath, runtime.ServeHTTP)

		s := &http.Server{
			Handler:      h,
			Addr:         net.JoinHostPort(hostAddr, hostPort),
			WriteTimeout: ioTimeout,
			ReadTimeout:  ioTimeout,
			IdleTimeout:  ioTimeout,
		}

		return s.ListenAndServe()
	},
}

func init() {
	serviceCmd.PersistentFlags().StringVarP(&hostPath, "path", "P", "/", "host path")
	serviceCmd.PersistentFlags().StringVarP(&hostAddr, "host", "H", "", "host address. If not specified, listens on all interfaces in dual-stack mode when available.")
	serviceCmd.PersistentFlags().StringVarP(&hostPort, "port", "p", "8443", "host port")
	serviceCmd.PersistentFlags().DurationVarP(&ioTimeout, "timeout", "t", 1*time.Second, "I/O timeout")
}
