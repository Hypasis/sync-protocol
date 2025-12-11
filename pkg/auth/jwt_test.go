package auth

import (
	"testing"
	"time"
)

func TestJWTManager_GenerateAndValidateToken(t *testing.T) {
	manager := NewJWTManager("test-secret", 1*time.Hour, "hypasis-test")

	// Test valid token generation
	token, err := manager.GenerateToken("user123", []string{RoleAdmin, RoleReader})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if token == "" {
		t.Error("Generated token is empty")
	}

	// Test token validation
	claims, err := manager.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	if claims.UserID != "user123" {
		t.Errorf("Expected UserID 'user123', got '%s'", claims.UserID)
	}

	if len(claims.Roles) != 2 {
		t.Errorf("Expected 2 roles, got %d", len(claims.Roles))
	}

	if claims.Issuer != "hypasis-test" {
		t.Errorf("Expected issuer 'hypasis-test', got '%s'", claims.Issuer)
	}
}

func TestJWTManager_InvalidToken(t *testing.T) {
	manager := NewJWTManager("test-secret", 1*time.Hour, "hypasis-test")

	tests := []struct {
		name  string
		token string
	}{
		{"Empty token", ""},
		{"Invalid format", "invalid.token.here"},
		{"Wrong signature", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.ValidateToken(tt.token)
			if err == nil {
				t.Error("Expected error for invalid token, got nil")
			}
		})
	}
}

func TestJWTManager_ExpiredToken(t *testing.T) {
	// Create manager with very short TTL
	manager := NewJWTManager("test-secret", 1*time.Millisecond, "hypasis-test")

	token, err := manager.GenerateToken("user123", []string{RoleReader})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	_, err = manager.ValidateToken(token)
	if err == nil {
		t.Error("Expected error for expired token, got nil")
	}
}

func TestJWTManager_WrongSecret(t *testing.T) {
	manager1 := NewJWTManager("secret1", 1*time.Hour, "hypasis-test")
	manager2 := NewJWTManager("secret2", 1*time.Hour, "hypasis-test")

	// Generate token with manager1
	token, err := manager1.GenerateToken("user123", []string{RoleReader})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Try to validate with manager2 (different secret)
	_, err = manager2.ValidateToken(token)
	if err == nil {
		t.Error("Expected error when validating token with wrong secret, got nil")
	}
}

func TestClaims_HasRole(t *testing.T) {
	claims := &Claims{
		UserID: "user123",
		Roles:  []string{RoleAdmin, RoleReader},
	}

	tests := []struct {
		role     string
		expected bool
	}{
		{RoleAdmin, true},
		{RoleReader, true},
		{RoleValidator, false},
		{RoleWriter, false},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := claims.HasRole(tt.role)
			if got != tt.expected {
				t.Errorf("HasRole(%s) = %v, want %v", tt.role, got, tt.expected)
			}
		})
	}
}

func TestClaims_HasAnyRole(t *testing.T) {
	claims := &Claims{
		UserID: "user123",
		Roles:  []string{RoleReader},
	}

	tests := []struct {
		name     string
		roles    []string
		expected bool
	}{
		{"Has one role", []string{RoleReader}, true},
		{"Has one of multiple", []string{RoleAdmin, RoleReader}, true},
		{"Has none", []string{RoleAdmin, RoleValidator}, false},
		{"Empty list", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := claims.HasAnyRole(tt.roles...)
			if got != tt.expected {
				t.Errorf("HasAnyRole(%v) = %v, want %v", tt.roles, got, tt.expected)
			}
		})
	}
}

func TestClaims_IsAdmin(t *testing.T) {
	tests := []struct {
		name     string
		roles    []string
		expected bool
	}{
		{"Admin role present", []string{RoleAdmin}, true},
		{"Admin with other roles", []string{RoleAdmin, RoleReader}, true},
		{"No admin role", []string{RoleReader, RoleValidator}, false},
		{"Empty roles", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &Claims{
				UserID: "user123",
				Roles:  tt.roles,
			}
			got := claims.IsAdmin()
			if got != tt.expected {
				t.Errorf("IsAdmin() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestJWTManager_MultipleTokens(t *testing.T) {
	manager := NewJWTManager("test-secret", 1*time.Hour, "hypasis-test")

	// Generate multiple tokens
	token1, err := manager.GenerateToken("user1", []string{RoleAdmin})
	if err != nil {
		t.Fatalf("Failed to generate token1: %v", err)
	}

	token2, err := manager.GenerateToken("user2", []string{RoleReader})
	if err != nil {
		t.Fatalf("Failed to generate token2: %v", err)
	}

	// Verify they're different
	if token1 == token2 {
		t.Error("Expected different tokens for different users")
	}

	// Validate both tokens
	claims1, err := manager.ValidateToken(token1)
	if err != nil {
		t.Fatalf("Failed to validate token1: %v", err)
	}

	claims2, err := manager.ValidateToken(token2)
	if err != nil {
		t.Fatalf("Failed to validate token2: %v", err)
	}

	if claims1.UserID != "user1" {
		t.Errorf("Expected UserID 'user1', got '%s'", claims1.UserID)
	}

	if claims2.UserID != "user2" {
		t.Errorf("Expected UserID 'user2', got '%s'", claims2.UserID)
	}
}

func BenchmarkJWTManager_GenerateToken(b *testing.B) {
	manager := NewJWTManager("test-secret", 1*time.Hour, "hypasis-test")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = manager.GenerateToken("user123", []string{RoleAdmin, RoleReader})
	}
}

func BenchmarkJWTManager_ValidateToken(b *testing.B) {
	manager := NewJWTManager("test-secret", 1*time.Hour, "hypasis-test")
	token, _ := manager.GenerateToken("user123", []string{RoleAdmin, RoleReader})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = manager.ValidateToken(token)
	}
}
