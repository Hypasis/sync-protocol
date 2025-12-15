package health

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Monitor tracks health of the sync service
type Monitor struct {
	config      *MonitorConfig
	checks      map[string]HealthCheck
	checksMutex sync.RWMutex
	status      atomic.Value // stores *HealthStatus
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// MonitorConfig configures the health monitor
type MonitorConfig struct {
	CheckInterval    time.Duration // How often to run health checks
	UnhealthyTimeout time.Duration // How long before marking unhealthy
	EnableAutoHeal   bool          // Attempt to auto-heal issues
}

// HealthStatus represents the overall health status
type HealthStatus struct {
	Healthy      bool                       `json:"healthy"`
	LastCheck    time.Time                  `json:"last_check"`
	Uptime       time.Duration              `json:"uptime"`
	StartTime    time.Time                  `json:"start_time"`
	CheckResults map[string]*CheckResult    `json:"check_results"`
	Metrics      map[string]interface{}     `json:"metrics"`
}

// CheckResult represents the result of a single health check
type CheckResult struct {
	Name        string        `json:"name"`
	Healthy     bool          `json:"healthy"`
	Message     string        `json:"message"`
	LastChecked time.Time     `json:"last_checked"`
	Duration    time.Duration `json:"duration"`
	ErrorCount  uint64        `json:"error_count"`
}

// HealthCheck interface for implementing health checks
type HealthCheck interface {
	Name() string
	Check(ctx context.Context) error
	Critical() bool // If true, failure marks entire service unhealthy
}

// NewMonitor creates a new health monitor
func NewMonitor(cfg *MonitorConfig) *Monitor {
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 10 * time.Second
	}
	if cfg.UnhealthyTimeout == 0 {
		cfg.UnhealthyTimeout = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Monitor{
		config: cfg,
		checks: make(map[string]HealthCheck),
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize status
	m.status.Store(&HealthStatus{
		Healthy:      true,
		StartTime:    time.Now(),
		CheckResults: make(map[string]*CheckResult),
		Metrics:      make(map[string]interface{}),
	})

	return m
}

// RegisterCheck registers a new health check
func (m *Monitor) RegisterCheck(check HealthCheck) {
	m.checksMutex.Lock()
	defer m.checksMutex.Unlock()
	m.checks[check.Name()] = check
	fmt.Printf("✅ Registered health check: %s\n", check.Name())
}

// Start starts the health monitor
func (m *Monitor) Start() {
	m.wg.Add(1)
	go m.runChecks()
	fmt.Println("🏥 Health monitor started")
}

// Stop stops the health monitor
func (m *Monitor) Stop() {
	m.cancel()
	m.wg.Wait()
	fmt.Println("🏥 Health monitor stopped")
}

// runChecks periodically runs all health checks
func (m *Monitor) runChecks() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	// Run checks immediately on start
	m.executeChecks()

	for {
		select {
		case <-ticker.C:
			m.executeChecks()
		case <-m.ctx.Done():
			return
		}
	}
}

// executeChecks runs all registered health checks
func (m *Monitor) executeChecks() {
	m.checksMutex.RLock()
	checks := make([]HealthCheck, 0, len(m.checks))
	for _, check := range m.checks {
		checks = append(checks, check)
	}
	m.checksMutex.RUnlock()

	results := make(map[string]*CheckResult)
	overallHealthy := true

	for _, check := range checks {
		result := m.runCheck(check)
		results[check.Name()] = result

		// If critical check fails, mark entire service unhealthy
		if check.Critical() && !result.Healthy {
			overallHealthy = false
		}
	}

	// Update status
	currentStatus := m.status.Load().(*HealthStatus)
	newStatus := &HealthStatus{
		Healthy:      overallHealthy,
		LastCheck:    time.Now(),
		Uptime:       time.Since(currentStatus.StartTime),
		StartTime:    currentStatus.StartTime,
		CheckResults: results,
		Metrics:      currentStatus.Metrics,
	}
	m.status.Store(newStatus)

	// Log unhealthy status
	if !overallHealthy {
		fmt.Printf("⚠️  Service unhealthy at %s\n", time.Now().Format(time.RFC3339))
		for name, result := range results {
			if !result.Healthy {
				fmt.Printf("   ❌ %s: %s\n", name, result.Message)
			}
		}

		// Attempt auto-heal if enabled
		if m.config.EnableAutoHeal {
			m.attemptAutoHeal(results)
		}
	}
}

// runCheck executes a single health check
func (m *Monitor) runCheck(check HealthCheck) *CheckResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	defer cancel()

	result := &CheckResult{
		Name:        check.Name(),
		LastChecked: start,
	}

	err := check.Check(ctx)
	result.Duration = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Message = err.Error()
		result.ErrorCount++
	} else {
		result.Healthy = true
		result.Message = "OK"
	}

	return result
}

// GetStatus returns the current health status
func (m *Monitor) GetStatus() *HealthStatus {
	status := m.status.Load().(*HealthStatus)
	return status
}

// IsHealthy returns true if the service is healthy
func (m *Monitor) IsHealthy() bool {
	status := m.status.Load().(*HealthStatus)
	return status.Healthy
}

// UpdateMetrics updates custom metrics
func (m *Monitor) UpdateMetrics(metrics map[string]interface{}) {
	status := m.status.Load().(*HealthStatus)
	newStatus := &HealthStatus{
		Healthy:      status.Healthy,
		LastCheck:    status.LastCheck,
		Uptime:       time.Since(status.StartTime),
		StartTime:    status.StartTime,
		CheckResults: status.CheckResults,
		Metrics:      metrics,
	}
	m.status.Store(newStatus)
}

// attemptAutoHeal attempts to automatically heal issues
func (m *Monitor) attemptAutoHeal(results map[string]*CheckResult) {
	// This is a placeholder for auto-healing logic
	// In production, you might:
	// - Restart failed components
	// - Clear caches
	// - Reconnect to services
	// - Alert operators

	for name, result := range results {
		if !result.Healthy && result.ErrorCount > 3 {
			fmt.Printf("🔧 Attempting to heal: %s\n", name)
			// Auto-heal logic would go here
		}
	}
}

// Common Health Checks

// StorageHealthCheck checks if storage is accessible
type StorageHealthCheck struct {
	storage interface {
		GetMaxBlock() uint64
	}
	critical bool
}

func NewStorageHealthCheck(storage interface{ GetMaxBlock() uint64 }) *StorageHealthCheck {
	return &StorageHealthCheck{storage: storage, critical: true}
}

func (c *StorageHealthCheck) Name() string {
	return "storage"
}

func (c *StorageHealthCheck) Critical() bool {
	return c.critical
}

func (c *StorageHealthCheck) Check(ctx context.Context) error {
	// Try to read max block
	maxBlock := c.storage.GetMaxBlock()
	if maxBlock == 0 {
		return fmt.Errorf("storage appears empty or inaccessible")
	}
	return nil
}

// CacheHealthCheck checks if cache (Redis) is accessible
type CacheHealthCheck struct {
	cache interface {
		HealthCheck(ctx context.Context) error
	}
	critical bool
}

func NewCacheHealthCheck(cache interface{ HealthCheck(ctx context.Context) error }) *CacheHealthCheck {
	return &CacheHealthCheck{cache: cache, critical: false} // Non-critical
}

func (c *CacheHealthCheck) Name() string {
	return "cache"
}

func (c *CacheHealthCheck) Critical() bool {
	return c.critical
}

func (c *CacheHealthCheck) Check(ctx context.Context) error {
	return c.cache.HealthCheck(ctx)
}

// SyncHealthCheck checks if sync is progressing
type SyncHealthCheck struct {
	coordinator interface {
		GetStatus() map[string]interface{}
	}
	critical bool
}

func NewSyncHealthCheck(coordinator interface{ GetStatus() map[string]interface{} }) *SyncHealthCheck {
	return &SyncHealthCheck{coordinator: coordinator, critical: true}
}

func (c *SyncHealthCheck) Name() string {
	return "sync"
}

func (c *SyncHealthCheck) Critical() bool {
	return c.critical
}

func (c *SyncHealthCheck) Check(ctx context.Context) error {
	status := c.coordinator.GetStatus()

	// Check if validator ready
	validatorReady, ok := status["validator_ready"].(bool)
	if !ok || !validatorReady {
		// Not critical if still syncing
		return nil
	}

	// Check forward sync
	forwardSync, ok := status["forward_sync"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("forward sync status unavailable")
	}

	blocksPerSec, _ := forwardSync["blocks_per_sec"].(float64)
	if blocksPerSec == 0 {
		return fmt.Errorf("forward sync appears stalled")
	}

	return nil
}

// P2PHealthCheck checks if P2P connections are healthy
type P2PHealthCheck struct {
	p2pServer interface {
		GetPeerCount() int
	}
	minPeers int
	critical bool
}

func NewP2PHealthCheck(p2pServer interface{ GetPeerCount() int }, minPeers int) *P2PHealthCheck {
	return &P2PHealthCheck{
		p2pServer: p2pServer,
		minPeers:  minPeers,
		critical:  false, // Non-critical
	}
}

func (c *P2PHealthCheck) Name() string {
	return "p2p"
}

func (c *P2PHealthCheck) Critical() bool {
	return c.critical
}

func (c *P2PHealthCheck) Check(ctx context.Context) error {
	peerCount := c.p2pServer.GetPeerCount()
	if peerCount == 0 {
		return fmt.Errorf("no peers connected")
	}
	if peerCount < c.minPeers {
		return fmt.Errorf("peer count (%d) below minimum (%d)", peerCount, c.minPeers)
	}
	return nil
}

// MemoryHealthCheck checks memory usage
type MemoryHealthCheck struct {
	maxMemoryMB uint64
	critical    bool
}

func NewMemoryHealthCheck(maxMemoryMB uint64) *MemoryHealthCheck {
	return &MemoryHealthCheck{
		maxMemoryMB: maxMemoryMB,
		critical:    false,
	}
}

func (c *MemoryHealthCheck) Name() string {
	return "memory"
}

func (c *MemoryHealthCheck) Critical() bool {
	return c.critical
}

func (c *MemoryHealthCheck) Check(ctx context.Context) error {
	// This would integrate with runtime.ReadMemStats() in production
	// For now, just return OK
	return nil
}
