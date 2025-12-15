package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter implements token bucket rate limiting per operator
type Limiter struct {
	config   *LimiterConfig
	limiters map[string]*rateLimiterEntry
	mu       sync.RWMutex
	global   *rate.Limiter
}

// LimiterConfig configures the rate limiter
type LimiterConfig struct {
	PerOperatorRPS int           // Requests per second per operator
	GlobalRPS      int           // Total requests per second (all operators)
	Burst          int           // Burst size for token bucket
	CleanupPeriod  time.Duration // How often to cleanup inactive limiters
}

// rateLimiterEntry represents a rate limiter for a single operator
type rateLimiterEntry struct {
	limiter      *rate.Limiter
	lastAccess   time.Time
	requestCount uint64
	blockedCount uint64
}

// NewLimiter creates a new rate limiter
func NewLimiter(cfg *LimiterConfig) *Limiter {
	if cfg.PerOperatorRPS <= 0 {
		cfg.PerOperatorRPS = 100 // Default: 100 RPS per operator
	}
	if cfg.GlobalRPS <= 0 {
		cfg.GlobalRPS = 100000 // Default: 100k RPS global
	}
	if cfg.Burst <= 0 {
		cfg.Burst = cfg.PerOperatorRPS * 2 // Default: 2x burst
	}
	if cfg.CleanupPeriod == 0 {
		cfg.CleanupPeriod = 5 * time.Minute
	}

	l := &Limiter{
		config:   cfg,
		limiters: make(map[string]*rateLimiterEntry),
		global:   rate.NewLimiter(rate.Limit(cfg.GlobalRPS), cfg.GlobalRPS),
	}

	// Start cleanup goroutine
	go l.cleanupLoop()

	return l
}

// Allow checks if a request from an operator is allowed
func (l *Limiter) Allow(operatorID string) bool {
	// Check global rate limit first
	if !l.global.Allow() {
		return false
	}

	// Get or create operator-specific limiter
	entry := l.getOrCreateLimiter(operatorID)
	entry.requestCount++

	// Check per-operator limit
	if !entry.limiter.Allow() {
		entry.blockedCount++
		return false
	}

	entry.lastAccess = time.Now()
	return true
}

// AllowN checks if N requests from an operator are allowed
func (l *Limiter) AllowN(operatorID string, n int) bool {
	// Check global rate limit
	if !l.global.AllowN(time.Now(), n) {
		return false
	}

	// Get or create operator-specific limiter
	entry := l.getOrCreateLimiter(operatorID)
	entry.requestCount += uint64(n)

	// Check per-operator limit
	if !entry.limiter.AllowN(time.Now(), n) {
		entry.blockedCount += uint64(n)
		return false
	}

	entry.lastAccess = time.Now()
	return true
}

// Wait waits until a request can proceed (blocking)
func (l *Limiter) Wait(ctx context.Context, operatorID string) error {
	// Wait on global limiter
	if err := l.global.Wait(ctx); err != nil {
		return err
	}

	// Wait on operator limiter
	entry := l.getOrCreateLimiter(operatorID)
	entry.requestCount++
	entry.lastAccess = time.Now()

	return entry.limiter.Wait(ctx)
}

// Reserve reserves capacity for a request (returns a reservation)
func (l *Limiter) Reserve(operatorID string) *rate.Reservation {
	entry := l.getOrCreateLimiter(operatorID)
	entry.requestCount++
	entry.lastAccess = time.Now()

	return entry.limiter.Reserve()
}

// getOrCreateLimiter gets or creates a rate limiter for an operator
func (l *Limiter) getOrCreateLimiter(operatorID string) *rateLimiterEntry {
	l.mu.RLock()
	entry, exists := l.limiters[operatorID]
	l.mu.RUnlock()

	if exists {
		return entry
	}

	// Create new limiter
	l.mu.Lock()
	defer l.mu.Unlock()

	// Double-check after acquiring write lock
	if entry, exists := l.limiters[operatorID]; exists {
		return entry
	}

	entry = &rateLimiterEntry{
		limiter:    rate.NewLimiter(rate.Limit(l.config.PerOperatorRPS), l.config.Burst),
		lastAccess: time.Now(),
	}
	l.limiters[operatorID] = entry

	return entry
}

// GetStats returns statistics for an operator
func (l *Limiter) GetStats(operatorID string) map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	entry, exists := l.limiters[operatorID]
	if !exists {
		return map[string]interface{}{
			"exists": false,
		}
	}

	return map[string]interface{}{
		"exists":        true,
		"request_count": entry.requestCount,
		"blocked_count": entry.blockedCount,
		"last_access":   entry.lastAccess.Unix(),
		"tokens":        entry.limiter.Tokens(),
		"limit":         l.config.PerOperatorRPS,
		"burst":         l.config.Burst,
	}
}

// GetAllStats returns statistics for all operators
func (l *Limiter) GetAllStats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	operatorStats := make(map[string]map[string]interface{})
	totalRequests := uint64(0)
	totalBlocked := uint64(0)

	for operatorID, entry := range l.limiters {
		operatorStats[operatorID] = map[string]interface{}{
			"request_count": entry.requestCount,
			"blocked_count": entry.blockedCount,
			"last_access":   entry.lastAccess.Unix(),
			"tokens":        entry.limiter.Tokens(),
		}
		totalRequests += entry.requestCount
		totalBlocked += entry.blockedCount
	}

	return map[string]interface{}{
		"total_operators":    len(l.limiters),
		"total_requests":     totalRequests,
		"total_blocked":      totalBlocked,
		"per_operator_limit": l.config.PerOperatorRPS,
		"global_limit":       l.config.GlobalRPS,
		"burst":              l.config.Burst,
		"global_tokens":      l.global.Tokens(),
		"operators":          operatorStats,
	}
}

// UpdateLimits updates rate limits (hot reload)
func (l *Limiter) UpdateLimits(perOperatorRPS, globalRPS int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.config.PerOperatorRPS = perOperatorRPS
	l.config.GlobalRPS = globalRPS

	// Update global limiter
	l.global.SetLimit(rate.Limit(globalRPS))
	l.global.SetBurst(globalRPS)

	// Update all operator limiters
	for _, entry := range l.limiters {
		entry.limiter.SetLimit(rate.Limit(perOperatorRPS))
		entry.limiter.SetBurst(l.config.Burst)
	}

	fmt.Printf("🔄 Rate limits updated: %d per operator, %d global\n", perOperatorRPS, globalRPS)
}

// RemoveOperator removes an operator's rate limiter
func (l *Limiter) RemoveOperator(operatorID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.limiters, operatorID)
}

// cleanupLoop periodically removes inactive limiters to save memory
func (l *Limiter) cleanupLoop() {
	ticker := time.NewTicker(l.config.CleanupPeriod)
	defer ticker.Stop()

	for range ticker.C {
		l.cleanup()
	}
}

// cleanup removes limiters that haven't been accessed recently
func (l *Limiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	inactiveThreshold := 30 * time.Minute
	removed := 0

	for operatorID, entry := range l.limiters {
		if now.Sub(entry.lastAccess) > inactiveThreshold {
			delete(l.limiters, operatorID)
			removed++
		}
	}

	if removed > 0 {
		fmt.Printf("🧹 Cleaned up %d inactive rate limiters\n", removed)
	}
}

// GetOperatorCount returns the number of tracked operators
func (l *Limiter) GetOperatorCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.limiters)
}

// IsOperatorLimited checks if an operator is currently rate-limited
func (l *Limiter) IsOperatorLimited(operatorID string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	entry, exists := l.limiters[operatorID]
	if !exists {
		return false
	}

	return entry.limiter.Tokens() < 1
}

// GetBlockedOperators returns a list of currently rate-limited operators
func (l *Limiter) GetBlockedOperators() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	blocked := make([]string, 0)
	for operatorID, entry := range l.limiters {
		if entry.limiter.Tokens() < 1 {
			blocked = append(blocked, operatorID)
		}
	}

	return blocked
}

// ResetOperator resets rate limit counters for an operator
func (l *Limiter) ResetOperator(operatorID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry, exists := l.limiters[operatorID]; exists {
		entry.requestCount = 0
		entry.blockedCount = 0
		entry.limiter = rate.NewLimiter(rate.Limit(l.config.PerOperatorRPS), l.config.Burst)
	}
}

// SetOperatorLimit sets a custom limit for a specific operator
func (l *Limiter) SetOperatorLimit(operatorID string, rps int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, exists := l.limiters[operatorID]
	if !exists {
		entry = &rateLimiterEntry{
			limiter:    rate.NewLimiter(rate.Limit(rps), rps*2),
			lastAccess: time.Now(),
		}
		l.limiters[operatorID] = entry
	} else {
		entry.limiter.SetLimit(rate.Limit(rps))
		entry.limiter.SetBurst(rps * 2)
	}

	fmt.Printf("🔧 Custom rate limit set for %s: %d RPS\n", operatorID, rps)
}
