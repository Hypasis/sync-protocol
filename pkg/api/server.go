package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/hypasis/sync-protocol/pkg/checkpoint"
	"github.com/hypasis/sync-protocol/pkg/config"
	"github.com/hypasis/sync-protocol/pkg/storage"
	"github.com/hypasis/sync-protocol/pkg/sync"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server represents the API server
type Server struct {
	config      *config.APIConfig
	coordinator *sync.Coordinator
	storage     storage.Storage
	checkpoint  *checkpoint.Manager

	restServer    *http.Server
	metricsServer *http.Server
}

// NewServer creates a new API server
func NewServer(
	cfg *config.APIConfig,
	coordinator *sync.Coordinator,
	storage storage.Storage,
	checkpointMgr *checkpoint.Manager,
) *Server {
	return &Server{
		config:     cfg,
		coordinator: coordinator,
		storage:     storage,
		checkpoint:  checkpointMgr,
	}
}

// Start starts the API server
func (s *Server) Start(ctx context.Context) error {
	// Start REST API
	if s.config.REST.Enabled {
		if err := s.startRESTServer(); err != nil {
			return fmt.Errorf("failed to start REST server: %w", err)
		}
	}

	// Start metrics server
	if s.config.Metrics.Enabled {
		if err := s.startMetricsServer(); err != nil {
			return fmt.Errorf("failed to start metrics server: %w", err)
		}
	}

	return nil
}

// Stop stops the API server
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.restServer != nil {
		s.restServer.Shutdown(ctx)
	}

	if s.metricsServer != nil {
		s.metricsServer.Shutdown(ctx)
	}

	return nil
}

// startRESTServer starts the REST API server
func (s *Server) startRESTServer() error {
	router := mux.NewRouter()

	// API v1 routes
	v1 := router.PathPrefix("/api/v1").Subrouter()

	v1.HandleFunc("/status", s.handleStatus).Methods("GET")
	v1.HandleFunc("/gaps", s.handleGaps).Methods("GET")
	v1.HandleFunc("/checkpoints", s.handleCheckpoints).Methods("GET")
	v1.HandleFunc("/sync/pause", s.handlePauseBackwardSync).Methods("POST")
	v1.HandleFunc("/sync/resume", s.handleResumeBackwardSync).Methods("POST")

	// Health check
	router.HandleFunc("/health", s.handleHealth).Methods("GET")

	// CORS
	if s.config.REST.CORS {
		router.Use(corsMiddleware)
	}

	s.restServer = &http.Server{
		Addr:    s.config.REST.Listen,
		Handler: router,
	}

	go func() {
		if err := s.restServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("REST server error: %v\n", err)
		}
	}()

	return nil
}

// startMetricsServer starts the Prometheus metrics server
func (s *Server) startMetricsServer() error {
	mux := http.NewServeMux()
	mux.Handle(s.config.Metrics.Path, promhttp.Handler())

	s.metricsServer = &http.Server{
		Addr:    s.config.Metrics.Listen,
		Handler: mux,
	}

	go func() {
		if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Metrics server error: %v\n", err)
		}
	}()

	return nil
}

// handleStatus handles GET /api/v1/status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := s.coordinator.GetStatus()
	respondJSON(w, http.StatusOK, status)
}

// handleGaps handles GET /api/v1/gaps
func (s *Server) handleGaps(w http.ResponseWriter, r *http.Request) {
	gaps := s.storage.GetGaps()

	response := map[string]interface{}{
		"missing_ranges": gaps,
		"count":          len(gaps),
	}

	respondJSON(w, http.StatusOK, response)
}

// handleCheckpoints handles GET /api/v1/checkpoints
func (s *Server) handleCheckpoints(w http.ResponseWriter, r *http.Request) {
	checkpoints := s.checkpoint.GetAll()

	response := map[string]interface{}{
		"checkpoints": checkpoints,
		"count":       len(checkpoints),
		"latest":      s.checkpoint.GetLatest(),
	}

	respondJSON(w, http.StatusOK, response)
}

// handlePauseBackwardSync handles POST /api/v1/sync/pause
func (s *Server) handlePauseBackwardSync(w http.ResponseWriter, r *http.Request) {
	if err := s.coordinator.PauseBackwardSync(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"status": "backward sync paused",
	})
}

// handleResumeBackwardSync handles POST /api/v1/sync/resume
func (s *Server) handleResumeBackwardSync(w http.ResponseWriter, r *http.Request) {
	if err := s.coordinator.ResumeBackwardSync(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"status": "backward sync resumed",
	})
}

// handleHealth handles GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
	})
}

// respondJSON sends a JSON response
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError sends an error response
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{
		"error": message,
	})
}

// corsMiddleware adds CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
