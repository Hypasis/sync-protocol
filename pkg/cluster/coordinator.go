package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Coordinator manages coordination between multiple Hypasis instances in a cluster
type Coordinator struct {
	config      *CoordinatorConfig
	cache       CacheInterface
	instanceID  string
	peers       map[string]*PeerInstance
	peersMutex  sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// CoordinatorConfig configures cluster coordination
type CoordinatorConfig struct {
	InstanceID       string        // Unique ID for this instance
	Region           string        // AWS region or datacenter
	HeartbeatPeriod  time.Duration // How often to send heartbeat
	PeerTimeout      time.Duration // When to consider peer dead
	EnableMeshSync   bool          // Enable block sync between instances
	LeaderElection   bool          // Enable leader election
}

// PeerInstance represents another Hypasis instance in the cluster
type PeerInstance struct {
	ID              string                 `json:"id"`
	Region          string                 `json:"region"`
	LastHeartbeat   time.Time              `json:"last_heartbeat"`
	Status          string                 `json:"status"` // "healthy", "degraded", "offline"
	Metrics         map[string]interface{} `json:"metrics"`
	ConnectedPeers  int                    `json:"connected_peers"`
	SyncProgress    float64                `json:"sync_progress"`
	IsLeader        bool                   `json:"is_leader"`
}

// CacheInterface defines the cache methods needed for coordination
type CacheInterface interface {
	GetSyncStatus(ctx context.Context, instanceID string) (map[string]interface{}, error)
	SetSyncStatus(ctx context.Context, instanceID string, status map[string]interface{}) error
	GetAllInstanceStatuses(ctx context.Context) (map[string]map[string]interface{}, error)
	PublishEvent(ctx context.Context, channel string, message interface{}) error
	SubscribeToEvents(ctx context.Context, channel string) (<-chan []byte, error)
}

// NewCoordinator creates a new cluster coordinator
func NewCoordinator(cfg *CoordinatorConfig, cache CacheInterface) *Coordinator {
	if cfg.HeartbeatPeriod == 0 {
		cfg.HeartbeatPeriod = 10 * time.Second
	}
	if cfg.PeerTimeout == 0 {
		cfg.PeerTimeout = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Coordinator{
		config:     cfg,
		cache:      cache,
		instanceID: cfg.InstanceID,
		peers:      make(map[string]*PeerInstance),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start starts the cluster coordinator
func (c *Coordinator) Start() error {
	fmt.Printf("🌐 Starting cluster coordinator: %s (%s)\n", c.instanceID, c.config.Region)

	// Start heartbeat sender
	c.wg.Add(1)
	go c.heartbeatLoop()

	// Start peer discovery
	c.wg.Add(1)
	go c.discoveryLoop()

	// Subscribe to cluster events
	eventCh, err := c.cache.SubscribeToEvents(c.ctx, "cluster:events")
	if err != nil {
		fmt.Printf("⚠️  Failed to subscribe to cluster events: %v\n", err)
	} else {
		c.wg.Add(1)
		go c.eventLoop(eventCh)
	}

	// Leader election if enabled
	if c.config.LeaderElection {
		c.wg.Add(1)
		go c.leaderElectionLoop()
	}

	return nil
}

// Stop stops the cluster coordinator
func (c *Coordinator) Stop() {
	c.cancel()
	c.wg.Wait()
	fmt.Println("🌐 Cluster coordinator stopped")
}

// heartbeatLoop sends periodic heartbeats
func (c *Coordinator) heartbeatLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.HeartbeatPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.sendHeartbeat()
		case <-c.ctx.Done():
			return
		}
	}
}

// sendHeartbeat sends a heartbeat with current status
func (c *Coordinator) sendHeartbeat() {
	status := map[string]interface{}{
		"instance_id":     c.instanceID,
		"region":          c.config.Region,
		"timestamp":       time.Now().Unix(),
		"status":          "healthy", // TODO: Get from health monitor
		"connected_peers": 0,          // TODO: Get from P2P server
		"sync_progress":   99.9,       // TODO: Get from sync coordinator
		"is_leader":       c.isLeader(),
	}

	if err := c.cache.SetSyncStatus(c.ctx, c.instanceID, status); err != nil {
		fmt.Printf("⚠️  Failed to send heartbeat: %v\n", err)
	}
}

// discoveryLoop discovers other instances in the cluster
func (c *Coordinator) discoveryLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.HeartbeatPeriod * 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.discoverPeers()
		case <-c.ctx.Done():
			return
		}
	}
}

// discoverPeers discovers other instances from cache
func (c *Coordinator) discoverPeers() {
	statuses, err := c.cache.GetAllInstanceStatuses(c.ctx)
	if err != nil {
		fmt.Printf("⚠️  Failed to discover peers: %v\n", err)
		return
	}

	c.peersMutex.Lock()
	defer c.peersMutex.Unlock()

	now := time.Now()

	for instanceID, status := range statuses {
		if instanceID == c.instanceID {
			continue // Skip self
		}

		// Parse timestamp
		timestampFloat, _ := status["timestamp"].(float64)
		timestamp := time.Unix(int64(timestampFloat), 0)

		// Check if peer is alive
		if now.Sub(timestamp) > c.config.PeerTimeout {
			if peer, exists := c.peers[instanceID]; exists {
				peer.Status = "offline"
			}
			continue
		}

		// Update or create peer
		peer := &PeerInstance{
			ID:            instanceID,
			LastHeartbeat: timestamp,
			Status:        "healthy",
			Metrics:       status,
		}

		if region, ok := status["region"].(string); ok {
			peer.Region = region
		}
		if connectedPeers, ok := status["connected_peers"].(float64); ok {
			peer.ConnectedPeers = int(connectedPeers)
		}
		if syncProgress, ok := status["sync_progress"].(float64); ok {
			peer.SyncProgress = syncProgress
		}
		if isLeader, ok := status["is_leader"].(bool); ok {
			peer.IsLeader = isLeader
		}

		c.peers[instanceID] = peer
	}
}

// eventLoop handles cluster events
func (c *Coordinator) eventLoop(eventCh <-chan []byte) {
	defer c.wg.Done()

	for {
		select {
		case event := <-eventCh:
			c.handleEvent(event)
		case <-c.ctx.Done():
			return
		}
	}
}

// handleEvent handles a cluster event
func (c *Coordinator) handleEvent(eventData []byte) {
	var event map[string]interface{}
	if err := json.Unmarshal(eventData, &event); err != nil {
		return
	}

	eventType, _ := event["type"].(string)
	sourceID, _ := event["source"].(string)

	if sourceID == c.instanceID {
		return // Ignore own events
	}

	switch eventType {
	case "instance_joined":
		fmt.Printf("🔔 Instance joined: %s\n", sourceID)
		c.discoverPeers()

	case "instance_left":
		fmt.Printf("🔔 Instance left: %s\n", sourceID)
		c.peersMutex.Lock()
		delete(c.peers, sourceID)
		c.peersMutex.Unlock()

	case "leader_changed":
		leaderID, _ := event["leader_id"].(string)
		fmt.Printf("🔔 New leader elected: %s\n", leaderID)

	case "block_synced":
		// Another instance synced a block - could use for coordination
		blockNum, _ := event["block_number"].(float64)
		fmt.Printf("📦 Block %d synced by %s\n", uint64(blockNum), sourceID)

	default:
		// Unknown event type
	}
}

// leaderElectionLoop performs leader election
func (c *Coordinator) leaderElectionLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.electLeader()
		case <-c.ctx.Done():
			return
		}
	}
}

// electLeader elects a leader based on instance ID (simple algorithm)
func (c *Coordinator) electLeader() {
	c.peersMutex.RLock()
	defer c.peersMutex.RUnlock()

	// Simple leader election: lowest instance ID wins
	leaderID := c.instanceID
	for id, peer := range c.peers {
		if peer.Status == "healthy" && id < leaderID {
			leaderID = id
		}
	}

	// If we're the leader, announce it
	if leaderID == c.instanceID && !c.isLeader() {
		c.announceLeader()
	}
}

// isLeader checks if this instance is the leader
func (c *Coordinator) isLeader() bool {
	c.peersMutex.RLock()
	defer c.peersMutex.RUnlock()

	leaderID := c.instanceID
	for id, peer := range c.peers {
		if peer.Status == "healthy" && id < leaderID {
			return false
		}
	}
	return true
}

// announceLeader announces this instance as leader
func (c *Coordinator) announceLeader() {
	event := map[string]interface{}{
		"type":      "leader_changed",
		"source":    c.instanceID,
		"leader_id": c.instanceID,
		"timestamp": time.Now().Unix(),
	}

	c.cache.PublishEvent(c.ctx, "cluster:events", event)
	fmt.Printf("👑 This instance is now the leader\n")
}

// GetPeers returns information about cluster peers
func (c *Coordinator) GetPeers() []*PeerInstance {
	c.peersMutex.RLock()
	defer c.peersMutex.RUnlock()

	peers := make([]*PeerInstance, 0, len(c.peers))
	for _, peer := range c.peers {
		peers = append(peers, peer)
	}
	return peers
}

// GetHealthyPeerCount returns the number of healthy peers
func (c *Coordinator) GetHealthyPeerCount() int {
	c.peersMutex.RLock()
	defer c.peersMutex.RUnlock()

	count := 0
	for _, peer := range c.peers {
		if peer.Status == "healthy" {
			count++
		}
	}
	return count
}

// GetClusterStatus returns overall cluster status
func (c *Coordinator) GetClusterStatus() map[string]interface{} {
	c.peersMutex.RLock()
	defer c.peersMutex.RUnlock()

	totalPeers := len(c.peers)
	healthyPeers := 0
	totalConnectedClients := 0

	peerList := make([]map[string]interface{}, 0, totalPeers)
	for _, peer := range c.peers {
		if peer.Status == "healthy" {
			healthyPeers++
		}
		totalConnectedClients += peer.ConnectedPeers

		peerList = append(peerList, map[string]interface{}{
			"id":              peer.ID,
			"region":          peer.Region,
			"status":          peer.Status,
			"connected_peers": peer.ConnectedPeers,
			"sync_progress":   peer.SyncProgress,
			"is_leader":       peer.IsLeader,
			"last_heartbeat":  peer.LastHeartbeat.Unix(),
		})
	}

	return map[string]interface{}{
		"instance_id":            c.instanceID,
		"region":                 c.config.Region,
		"is_leader":              c.isLeader(),
		"total_instances":        totalPeers + 1, // +1 for self
		"healthy_instances":      healthyPeers,
		"total_connected_clients": totalConnectedClients,
		"peers":                  peerList,
	}
}

// PublishBlockSynced publishes an event when a block is synced
func (c *Coordinator) PublishBlockSynced(blockNum uint64) {
	event := map[string]interface{}{
		"type":         "block_synced",
		"source":       c.instanceID,
		"block_number": blockNum,
		"timestamp":    time.Now().Unix(),
	}

	c.cache.PublishEvent(c.ctx, "cluster:events", event)
}

// GetLeader returns the current cluster leader ID
func (c *Coordinator) GetLeader() string {
	c.peersMutex.RLock()
	defer c.peersMutex.RUnlock()

	leaderID := c.instanceID
	for id, peer := range c.peers {
		if peer.Status == "healthy" && peer.IsLeader {
			return id
		}
		if peer.Status == "healthy" && id < leaderID {
			leaderID = id
		}
	}
	return leaderID
}
