package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/hypasis/sync-protocol/pkg/auth"
	"github.com/hypasis/sync-protocol/pkg/checkpoint"
	"github.com/hypasis/sync-protocol/pkg/config"
	"github.com/hypasis/sync-protocol/pkg/storage"
	"github.com/hypasis/sync-protocol/pkg/sync"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// insecureJWTSecrets are placeholder secrets that must never be used with auth
// enabled. Starting with auth on and one of these is refused.
var insecureJWTSecrets = map[string]bool{
	"":                                 true,
	"change-this-secret":               true,
	"change-this-secret-in-production": true,
}

// Server represents the API server
type Server struct {
	config      *config.APIConfig
	coordinator *sync.Coordinator
	storage     storage.Storage
	checkpoint  *checkpoint.Manager

	jwtManager  *auth.JWTManager
	rateLimiter *auth.PerIPRateLimiter

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
	s := &Server{
		config:      cfg,
		coordinator: coordinator,
		storage:     storage,
		checkpoint:  checkpointMgr,
	}

	// Build the JWT manager when authentication is enabled.
	if cfg.REST.Auth.Enabled {
		ttl, err := time.ParseDuration(cfg.REST.Auth.TokenTTL)
		if err != nil || ttl <= 0 {
			ttl = 24 * time.Hour
		}
		s.jwtManager = auth.NewJWTManager(cfg.REST.Auth.JWTSecret, ttl, "hypasis-sync")
	}

	// Build a per-IP rate limiter when a positive limit is configured.
	if cfg.REST.RateLimit > 0 {
		burst := cfg.REST.RateLimit * 2
		s.rateLimiter = auth.NewPerIPRateLimiter(cfg.REST.RateLimit, burst, 10*time.Minute)
	}

	return s
}

// Start starts the API server
func (s *Server) Start(ctx context.Context) error {
	// Refuse to start with authentication enabled but an insecure secret.
	if s.config.REST.Auth.Enabled && insecureJWTSecrets[s.config.REST.Auth.JWTSecret] {
		return fmt.Errorf("auth is enabled but jwt_secret is empty or a known placeholder; set a strong secret")
	}

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

	// Global middleware (outermost first): request logging, config-aware CORS,
	// then per-IP rate limiting. These apply to every route, including /health.
	router.Use(auth.LoggingMiddleware())
	if s.config.REST.CORS {
		router.Use(auth.CORSMiddleware(s.config.REST.CORSOrigins, false))
	}
	if s.rateLimiter != nil {
		router.Use(s.rateLimiter.Middleware())
	}

	// Health check (public, no auth).
	router.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Token issuance (public) — only exposed when auth is enabled. Registered
	// before the protected subrouter so it is matched first.
	if s.jwtManager != nil {
		router.HandleFunc("/api/v1/auth/token", s.handleToken).Methods("POST")
	}

	// API v1 routes. When auth is enabled, everything under here requires a
	// valid JWT; state-changing routes additionally require a privileged role.
	v1 := router.PathPrefix("/api/v1").Subrouter()
	if s.jwtManager != nil {
		v1.Use(auth.AuthMiddleware(s.jwtManager))
	}

	v1.HandleFunc("/status", s.handleStatus).Methods("GET")
	v1.HandleFunc("/gaps", s.handleGaps).Methods("GET")
	v1.HandleFunc("/checkpoints", s.handleCheckpoints).Methods("GET")

	// State-changing endpoints: require writer/admin role when auth is enabled.
	pause := http.HandlerFunc(s.handlePauseBackwardSync)
	resume := http.HandlerFunc(s.handleResumeBackwardSync)
	if s.jwtManager != nil {
		requireWriter := auth.RequireRoleMiddleware(auth.RoleAdmin, auth.RoleWriter)
		v1.Handle("/sync/pause", requireWriter(pause)).Methods("POST")
		v1.Handle("/sync/resume", requireWriter(resume)).Methods("POST")
	} else {
		v1.Handle("/sync/pause", pause).Methods("POST")
		v1.Handle("/sync/resume", resume).Methods("POST")
	}

	s.restServer = &http.Server{
		Addr:    s.config.REST.Listen,
		Handler: router,
	}

	// TLS termination in-process when configured.
	if s.config.REST.TLS.Enabled {
		tlsCfg, err := auth.LoadTLSConfig(s.config.REST.TLS.CertFile, s.config.REST.TLS.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load TLS config: %w", err)
		}
		s.restServer.TLSConfig = tlsCfg

		go func() {
			// Cert/key already loaded into TLSConfig, so the file args are empty.
			if err := s.restServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				fmt.Printf("REST server (TLS) error: %v\n", err)
			}
		}()
		return nil
	}

	go func() {
		if err := s.restServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("REST server error: %v\n", err)
		}
	}()

	return nil
}

// tokenRequest is the body for POST /api/v1/auth/token.
type tokenRequest struct {
	UserID string   `json:"user_id"`
	Roles  []string `json:"roles"`
}

// handleToken issues a JWT for the requested user/roles. NOTE: this endpoint is
// intentionally open so operators can bootstrap a token; in a real deployment
// it must be placed behind admin credentials or an out-of-band issuance flow.
func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" {
		respondError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if len(req.Roles) == 0 {
		req.Roles = []string{auth.RoleReader}
	}

	token, err := s.jwtManager.GenerateToken(req.UserID, req.Roles)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"roles": req.Roles,
	})
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
