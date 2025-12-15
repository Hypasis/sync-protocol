package p2p

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/hypasis/sync-protocol/pkg/storage"
)

// DevP2PServer implements an Ethereum DevP2P server that acts as a bootnode
// for Bor clients, providing fast sync capabilities
type DevP2PServer struct {
	config      *DevP2PConfig
	storage     storage.Storage
	fetcher     *BlockFetcher
	server      *p2p.Server
	nodeKey     *ecdsa.PrivateKey
	localNode   *enode.LocalNode
	connections map[enode.ID]*peerConnection
	connMutex   sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// DevP2PConfig configures the DevP2P server
type DevP2PConfig struct {
	ListenAddr   string   // Listen address (e.g., "0.0.0.0:30303")
	ExternalIP   string   // External IP for enode URL (auto-detect if empty)
	MaxPeers     int      // Maximum number of connected peers
	NodeKey      string   // Path to node key file (generated if empty)
	NetworkID    uint64   // Network ID (137 for Polygon mainnet)
	Name         string   // Node name
	Bootnodes    []string // Bootstrap nodes (for connecting to other Hypasis instances)
	StaticPeers  []string // Static peers (other Hypasis instances in cluster)
	EnableNAT    bool     // Enable NAT traversal
}

// peerConnection represents a connected peer
type peerConnection struct {
	peer      *p2p.Peer
	rw        p2p.MsgReadWriter
	connected time.Time
	reqCount  uint64
	lastReq   time.Time
}

// NewDevP2PServer creates a new DevP2P server
func NewDevP2PServer(cfg *DevP2PConfig, store storage.Storage, fetcher *BlockFetcher) (*DevP2PServer, error) {
	// Load or generate node key
	var nodeKey *ecdsa.PrivateKey
	var err error

	if cfg.NodeKey != "" {
		nodeKey, err = crypto.LoadECDSA(cfg.NodeKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load node key: %w", err)
		}
	} else {
		nodeKey, err = crypto.GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate node key: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	server := &DevP2PServer{
		config:      cfg,
		storage:     store,
		fetcher:     fetcher,
		nodeKey:     nodeKey,
		connections: make(map[enode.ID]*peerConnection),
		ctx:         ctx,
		cancel:      cancel,
	}

	return server, nil
}

// Start starts the DevP2P server
func (s *DevP2PServer) Start() error {
	// Create local node database
	db, err := enode.OpenDB("")
	if err != nil {
		return fmt.Errorf("failed to open node database: %w", err)
	}

	// Parse listen address
	host, port, err := net.SplitHostPort(s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}

	// Convert port to int
	var listenPort int
	fmt.Sscanf(port, "%d", &listenPort)

	// Create local node
	s.localNode = enode.NewLocalNode(db, s.nodeKey)
	s.localNode.Set(enr.IP(net.ParseIP(host)))
	s.localNode.Set(enr.TCP(listenPort))
	s.localNode.Set(enr.UDP(listenPort))

	// Parse bootnodes
	var bootnodes []*enode.Node
	for _, bootnode := range s.config.Bootnodes {
		node, err := enode.Parse(enode.ValidSchemes, bootnode)
		if err != nil {
			fmt.Printf("Warning: invalid bootnode %s: %v\n", bootnode, err)
			continue
		}
		bootnodes = append(bootnodes, node)
	}

	// Parse static peers
	var staticPeers []*enode.Node
	for _, peer := range s.config.StaticPeers {
		node, err := enode.Parse(enode.ValidSchemes, peer)
		if err != nil {
			fmt.Printf("Warning: invalid static peer %s: %v\n", peer, err)
			continue
		}
		staticPeers = append(staticPeers, node)
	}

	// Configure NAT
	natManager := nat.Any()
	if !s.config.EnableNAT {
		natManager = nil
	}

	// Create P2P server config
	serverConfig := &p2p.Config{
		PrivateKey:      s.nodeKey,
		MaxPeers:        s.config.MaxPeers,
		Name:            s.config.Name,
		BootstrapNodes:  bootnodes,
		StaticNodes:     staticPeers,
		ListenAddr:      s.config.ListenAddr,
		NAT:             natManager,
		Protocols:       s.makeProtocols(),
		NodeDatabase:    "",
		EnableMsgEvents: true,
	}

	// Create and start P2P server
	s.server = &p2p.Server{Config: *serverConfig}

	if err := s.server.Start(); err != nil {
		return fmt.Errorf("failed to start P2P server: %w", err)
	}

	// Get external IP if not configured
	if s.config.ExternalIP == "" {
		s.config.ExternalIP = s.detectExternalIP()
	}

	// Print enode URL
	fmt.Printf("🌐 DevP2P server started\n")
	fmt.Printf("📍 Enode URL: %s\n", s.GetEnodeURL())
	fmt.Printf("🔌 Listening on: %s\n", s.config.ListenAddr)
	fmt.Printf("👥 Max peers: %d\n", s.config.MaxPeers)

	// Start background tasks
	s.wg.Add(1)
	go s.statsReporter()

	return nil
}

// Stop stops the DevP2P server
func (s *DevP2PServer) Stop() error {
	s.cancel()

	if s.server != nil {
		s.server.Stop()
	}

	s.wg.Wait()
	return nil
}

// GetEnodeURL returns the enode URL for this server
func (s *DevP2PServer) GetEnodeURL() string {
	node := s.server.Self()
	if s.config.ExternalIP != "" {
		// Override IP with external IP
		node = enode.NewV4(&s.nodeKey.PublicKey,
			net.ParseIP(s.config.ExternalIP),
			node.TCP(),
			node.UDP())
	}
	return node.URLv4()
}

// GetPeerCount returns the number of connected peers
func (s *DevP2PServer) GetPeerCount() int {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()
	return len(s.connections)
}

// makeProtocols creates the protocol handlers
func (s *DevP2PServer) makeProtocols() []p2p.Protocol {
	return []p2p.Protocol{
		{
			Name:    "eth",
			Version: 66, // eth/66 protocol
			Length:  17,
			Run: func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
				return s.handlePeer(peer, rw)
			},
			NodeInfo: func() interface{} {
				return map[string]interface{}{
					"network": s.config.NetworkID,
					"name":    s.config.Name,
					"version": "hypasis/v1.0.0",
				}
			},
			PeerInfo: func(id enode.ID) interface{} {
				s.connMutex.RLock()
				defer s.connMutex.RUnlock()
				if conn, ok := s.connections[id]; ok {
					return map[string]interface{}{
						"connected": conn.connected.Unix(),
						"requests":  conn.reqCount,
					}
				}
				return nil
			},
		},
	}
}

// handlePeer handles a connected peer
func (s *DevP2PServer) handlePeer(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
	// Register connection
	conn := &peerConnection{
		peer:      peer,
		rw:        rw,
		connected: time.Now(),
	}

	s.connMutex.Lock()
	s.connections[peer.ID()] = conn
	s.connMutex.Unlock()

	fmt.Printf("✅ Peer connected: %s (%s)\n", peer.Name(), peer.RemoteAddr())

	// Cleanup on disconnect
	defer func() {
		s.connMutex.Lock()
		delete(s.connections, peer.ID())
		s.connMutex.Unlock()
		fmt.Printf("👋 Peer disconnected: %s\n", peer.Name())
	}()

	// Send status message
	if err := s.sendStatus(rw); err != nil {
		return fmt.Errorf("failed to send status: %w", err)
	}

	// Handle messages
	for {
		msg, err := rw.ReadMsg()
		if err != nil {
			return err
		}

		conn.reqCount++
		conn.lastReq = time.Now()

		if err := s.handleMessage(peer, rw, msg); err != nil {
			msg.Discard()
			return err
		}
		msg.Discard()
	}
}

// sendStatus sends the status message to a peer
func (s *DevP2PServer) sendStatus(rw p2p.MsgReadWriter) error {
	maxBlock := s.storage.GetMaxBlock()
	minBlock := s.storage.GetMinBlock()

	// Get latest block hash
	ctx := context.Background()
	block, err := s.storage.GetBlock(ctx, maxBlock)
	var headHash [32]byte
	if err == nil && block != nil {
		copy(headHash[:], block.Header.Hash[:])
	}

	status := map[string]interface{}{
		"protocolVersion": uint32(66),
		"networkId":       s.config.NetworkID,
		"td":              uint64(0), // Total difficulty (not used in PoS)
		"bestHash":        headHash,
		"genesisHash":     [32]byte{}, // TODO: Get from config
		"head":            maxBlock,
		"min":             minBlock,
	}

	return p2p.Send(rw, 0x00, status) // 0x00 = Status message
}

// handleMessage handles incoming messages from peers
func (s *DevP2PServer) handleMessage(peer *p2p.Peer, rw p2p.MsgReadWriter, msg p2p.Msg) error {
	switch msg.Code {
	case 0x00: // Status
		// Decode and log status
		var status map[string]interface{}
		if err := msg.Decode(&status); err != nil {
			return err
		}
		fmt.Printf("📊 Received status from %s: %v\n", peer.Name(), status)

	case 0x03: // GetBlockHeaders
		return s.handleGetBlockHeaders(rw, msg)

	case 0x05: // GetBlockBodies
		return s.handleGetBlockBodies(rw, msg)

	case 0x0d: // GetPooledTransactions
		// Not implemented - send empty response
		return p2p.Send(rw, 0x0e, []interface{}{})

	default:
		fmt.Printf("⚠️  Unknown message code: 0x%x from %s\n", msg.Code, peer.Name())
	}

	return nil
}

// handleGetBlockHeaders handles block header requests
func (s *DevP2PServer) handleGetBlockHeaders(rw p2p.MsgReadWriter, msg p2p.Msg) error {
	// Decode request
	var req struct {
		Origin  uint64
		Amount  uint64
		Skip    uint64
		Reverse bool
	}
	if err := msg.Decode(&req); err != nil {
		return err
	}

	// Fetch headers (simplified - real implementation would be more complex)
	headers := []interface{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := uint64(0); i < req.Amount && i < 192; i++ { // Max 192 headers per response
		blockNum := req.Origin + i*req.Skip
		block, err := s.storage.GetBlock(ctx, blockNum)
		if err != nil {
			break
		}
		headers = append(headers, block.Header)
	}

	return p2p.Send(rw, 0x04, headers) // 0x04 = BlockHeaders
}

// handleGetBlockBodies handles block body requests
func (s *DevP2PServer) handleGetBlockBodies(rw p2p.MsgReadWriter, msg p2p.Msg) error {
	// Decode request (list of block hashes)
	var hashes [][]byte
	if err := msg.Decode(&hashes); err != nil {
		return err
	}

	// Fetch bodies (simplified)
	bodies := []interface{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, hash := range hashes {
		if len(hash) != 32 {
			continue
		}
		var h [32]byte
		copy(h[:], hash)

		block, err := s.storage.GetBlockByHash(ctx, h)
		if err != nil {
			continue
		}

		bodies = append(bodies, map[string]interface{}{
			"transactions": block.Transactions,
			"uncles":       []interface{}{}, // Polygon doesn't use uncles
		})

		if len(bodies) >= 128 { // Max 128 bodies per response
			break
		}
	}

	return p2p.Send(rw, 0x06, bodies) // 0x06 = BlockBodies
}

// detectExternalIP attempts to detect the external IP address
func (s *DevP2PServer) detectExternalIP() string {
	// Try to use NAT to get external IP
	if s.server != nil && s.server.Config.NAT != nil {
		if extIP, err := s.server.Config.NAT.ExternalIP(); err == nil {
			return extIP.String()
		}
	}

	// Fallback to "auto" marker
	return "auto"
}

// statsReporter periodically reports connection statistics
func (s *DevP2PServer) statsReporter() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.connMutex.RLock()
			peerCount := len(s.connections)
			s.connMutex.RUnlock()

			if peerCount > 0 {
				fmt.Printf("📊 DevP2P Stats: %d peers connected\n", peerCount)
			}

		case <-s.ctx.Done():
			return
		}
	}
}

// GetConnectedPeers returns information about connected peers
func (s *DevP2PServer) GetConnectedPeers() []map[string]interface{} {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	peers := make([]map[string]interface{}, 0, len(s.connections))
	for id, conn := range s.connections {
		peers = append(peers, map[string]interface{}{
			"id":        id.String(),
			"name":      conn.peer.Name(),
			"addr":      conn.peer.RemoteAddr().String(),
			"connected": conn.connected.Unix(),
			"requests":  conn.reqCount,
		})
	}

	return peers
}
