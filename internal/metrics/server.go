package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

// Server wraps the HTTP server for Prometheus metrics
type Server struct {
	server *http.Server
	port   int
}

// NewServer creates a new metrics server
func NewServer(port int) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	// Health check endpoint
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Readiness check endpoint
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	})

	return &Server{
		server: &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		port: port,
	}
}

// Start starts the metrics server in the background
func (s *Server) Start() error {
	klog.InfoS("Starting metrics server", "port", s.port)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			klog.ErrorS(err, "Metrics server failed")
		}
	}()

	return nil
}

// Shutdown gracefully shuts down the metrics server
func (s *Server) Shutdown(ctx context.Context) error {
	klog.Info("Shutting down metrics server")
	return s.server.Shutdown(ctx)
}
