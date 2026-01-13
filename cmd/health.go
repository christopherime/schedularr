package cmd

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Start a health check endpoint",
	Long: `Starts a simple HTTP server to expose health check endpoints.
Useful for Docker/Kubernetes readiness and liveness probes.

Endpoints:
  /healthz - Returns 200 OK if the application is healthy.
  /livez   - Returns 200 OK if the application is alive.

Example:
  schedularr health --port 8080`,
	Run: func(_ *cobra.Command, _ []string) {
		startHealthCheckServer(healthPort)
	},
}

var healthPort int

func init() {
	rootCmd.AddCommand(healthCmd)
	healthCmd.Flags().IntVarP(&healthPort, "port", "p", 9600, "Port to listen on for health checks")
}

func startHealthCheckServer(port int) {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "OK")
	})

	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "OK")
	})

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Health check server listening on %s\n", addr)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second, // Prevent slowloris attacks
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "%s Health check server failed: %v\n", errorStyle.Render("✗"), err)
		os.Exit(1)
	}
}
