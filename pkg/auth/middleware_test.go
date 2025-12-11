package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthMiddleware(t *testing.T) {
	manager := NewJWTManager("test-secret", 1*time.Hour, "hypasis-test")
	middleware := AuthMiddleware(manager)

	// Generate valid token
	validToken, err := manager.GenerateToken("user123", []string{RoleReader})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "Valid token",
			authHeader:     "Bearer " + validToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Missing authorization header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid format",
			authHeader:     "InvalidFormat",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Wrong prefix",
			authHeader:     "Basic " + validToken,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid token",
			authHeader:     "Bearer invalid.token.here",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()

			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestOptionalAuthMiddleware(t *testing.T) {
	manager := NewJWTManager("test-secret", 1*time.Hour, "hypasis-test")
	middleware := OptionalAuthMiddleware(manager)

	validToken, _ := manager.GenerateToken("user123", []string{RoleReader})

	tests := []struct {
		name       string
		authHeader string
		shouldPass bool
	}{
		{
			name:       "With valid token",
			authHeader: "Bearer " + validToken,
			shouldPass: true,
		},
		{
			name:       "Without token",
			authHeader: "",
			shouldPass: true,
		},
		{
			name:       "With invalid token",
			authHeader: "Bearer invalid",
			shouldPass: true, // Should still pass, just without claims
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()

			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected status OK, got %d", rr.Code)
			}
		})
	}
}

func TestRequireRoleMiddleware(t *testing.T) {
	manager := NewJWTManager("test-secret", 1*time.Hour, "hypasis-test")
	authMW := AuthMiddleware(manager)
	roleMW := RequireRoleMiddleware(RoleAdmin)

	adminToken, _ := manager.GenerateToken("admin", []string{RoleAdmin})
	readerToken, _ := manager.GenerateToken("reader", []string{RoleReader})

	tests := []struct {
		name           string
		token          string
		expectedStatus int
	}{
		{
			name:           "Admin has access",
			token:          adminToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Reader denied",
			token:          readerToken,
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/admin", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)

			rr := httptest.NewRecorder()

			handler := authMW(roleMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})))

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	middleware := RateLimitMiddleware(2) // 2 requests per second

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// The rate limiter has a burst capacity of 2x the rate (4 requests)
	// So first 4 requests should succeed
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected status OK, got %d", i+1, rr.Code)
		}
	}

	// Fifth request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status TooManyRequests, got %d", rr.Code)
	}
}

func TestPerIPRateLimiter(t *testing.T) {
	limiter := NewPerIPRateLimiter(2, 2, 1*time.Minute)
	middleware := limiter.Middleware()

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Requests from different IPs should not affect each other
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Errorf("Request from IP1: expected OK, got %d", rr1.Code)
	}

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.2:12346"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Request from IP2: expected OK, got %d", rr2.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	tests := []struct {
		name            string
		allowedOrigins  []string
		requestOrigin   string
		expectAllowed   bool
	}{
		{
			name:            "Allow all origins",
			allowedOrigins:  []string{"*"},
			requestOrigin:   "https://example.com",
			expectAllowed:   true,
		},
		{
			name:            "Specific origin allowed",
			allowedOrigins:  []string{"https://example.com"},
			requestOrigin:   "https://example.com",
			expectAllowed:   true,
		},
		{
			name:            "Origin not allowed",
			allowedOrigins:  []string{"https://example.com"},
			requestOrigin:   "https://evil.com",
			expectAllowed:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := CORSMiddleware(tt.allowedOrigins, false)
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Origin", tt.requestOrigin)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			corsHeader := rr.Header().Get("Access-Control-Allow-Origin")
			if tt.expectAllowed && corsHeader == "" {
				t.Error("Expected CORS header, got none")
			}
			if !tt.expectAllowed && corsHeader != "" && corsHeader != tt.requestOrigin {
				t.Errorf("Did not expect CORS header for origin %s", tt.requestOrigin)
			}
		})
	}
}

func TestCORSMiddleware_Preflight(t *testing.T) {
	middleware := CORSMiddleware([]string{"*"}, false)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for OPTIONS request")
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status OK for preflight, got %d", rr.Code)
	}

	if rr.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("Expected CORS headers in preflight response")
	}
}

func TestGetClaims(t *testing.T) {
	manager := NewJWTManager("test-secret", 1*time.Hour, "hypasis-test")
	authMW := AuthMiddleware(manager)

	token, _ := manager.GenerateToken("user123", []string{RoleReader})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()

	handler := authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r.Context())
		if claims == nil {
			t.Error("Expected claims in context, got nil")
		} else if claims.UserID != "user123" {
			t.Errorf("Expected UserID 'user123', got '%s'", claims.UserID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		realIP     string
		expected   string
	}{
		{
			name:       "Direct connection",
			remoteAddr: "192.168.1.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For",
			remoteAddr: "127.0.0.1:12345",
			forwarded:  "203.0.113.1, 198.51.100.1",
			expected:   "203.0.113.1",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "127.0.0.1:12345",
			realIP:     "203.0.113.1",
			expected:   "203.0.113.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}

			got := getClientIP(req)
			if got != tt.expected {
				t.Errorf("Expected IP %s, got %s", tt.expected, got)
			}
		})
	}
}

func BenchmarkAuthMiddleware(b *testing.B) {
	manager := NewJWTManager("test-secret", 1*time.Hour, "hypasis-test")
	middleware := AuthMiddleware(manager)
	token, _ := manager.GenerateToken("user123", []string{RoleReader})

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkRateLimitMiddleware(b *testing.B) {
	middleware := RateLimitMiddleware(1000000) // High limit for benchmarking

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}
