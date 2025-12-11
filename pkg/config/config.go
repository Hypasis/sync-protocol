package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete Hypasis configuration
type Config struct {
	Chain      ChainConfig      `yaml:"chain"`
	Checkpoint CheckpointConfig `yaml:"checkpoint"`
	Sync       SyncConfig       `yaml:"sync"`
	Storage    StorageConfig    `yaml:"storage"`
	P2P        P2PConfig        `yaml:"p2p"`
	API        APIConfig        `yaml:"api"`
	Logging    LoggingConfig    `yaml:"logging"`
}

// ChainConfig contains blockchain-specific settings
type ChainConfig struct {
	Name    string `yaml:"name"`
	Network string `yaml:"network"` // mainnet, testnet, etc.
	ChainID int64  `yaml:"chain_id"`
}

// CheckpointConfig contains checkpoint system settings
type CheckpointConfig struct {
	Source   string        `yaml:"source"`   // ethereum-l1, mock, beacon-chain, native
	Contract string        `yaml:"contract"` // Smart contract address
	Interval uint64        `yaml:"interval"` // Checkpoint interval in blocks
	Timeout  time.Duration `yaml:"timeout"`

	// L1 Connection (for ethereum-l1 source)
	L1RPCURL     string `yaml:"l1_rpc_url"`      // Ethereum L1 RPC URL (Infura, Alchemy, etc.)
	L1ChainID    int64  `yaml:"l1_chain_id"`     // 1 for mainnet, 5 for Goerli, 11155111 for Sepolia
	ValidatorRPC string `yaml:"validator_rpc"`   // Heimdall API for validator set (optional)
}

// SyncConfig contains synchronization settings
type SyncConfig struct {
	Forward  ForwardSyncConfig  `yaml:"forward"`
	Backward BackwardSyncConfig `yaml:"backward"`
}

// ForwardSyncConfig contains forward sync settings
type ForwardSyncConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Workers         int    `yaml:"workers"`
	LookAhead       int    `yaml:"look_ahead"`       // Pre-fetch N blocks ahead
	ValidationLevel string `yaml:"validation_level"` // none, header, light, full
}

// BackwardSyncConfig contains backward sync settings
type BackwardSyncConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Workers         int    `yaml:"workers"`
	BatchSize       int    `yaml:"batch_size"`       // Blocks per batch
	Throttle        string `yaml:"throttle"`         // e.g., "50MB/s"
	ValidationLevel string `yaml:"validation_level"` // none, header, light, full
}

// StorageConfig contains storage settings
type StorageConfig struct {
	DataDir         string `yaml:"data_dir"`
	CacheSize       string `yaml:"cache_size"`          // e.g., "2GB" (for PebbleDB cache)
	WriteBufferSize string `yaml:"write_buffer_size"`   // e.g., "256MB" (for PebbleDB write buffer)
	MaxOpenFiles    int    `yaml:"max_open_files"`      // Max open file descriptors (default: 1024)
	HistoricalDepth string `yaml:"historical_depth"`    // full, 1year, 90days, minimal
	Engine          string `yaml:"engine"`              // memory, pebble, leveldb, badger
}

// P2PConfig contains P2P networking settings
type P2PConfig struct {
	Mode         string        `yaml:"mode"`          // "rpc-server" or "devp2p"
	Listen       string        `yaml:"listen"`        // Legacy devp2p listen address
	RPCListen    string        `yaml:"rpc_listen"`    // RPC server listen address (e.g., "0.0.0.0:8545")
	UpstreamRPCs []string      `yaml:"upstream_rpcs"` // Upstream RPC endpoints to fetch blocks
	RPCTimeout   time.Duration `yaml:"rpc_timeout"`   // RPC request timeout
	RPCMaxRetries int          `yaml:"rpc_max_retries"` // Max retries for RPC requests
	Bootnodes    []string      `yaml:"bootnodes"`     // Legacy bootnodes
	MaxPeers     int           `yaml:"max_peers"`     // Legacy max peers
	StaticPeers  []string      `yaml:"static_peers"`  // Legacy static peers
	NAT          bool          `yaml:"nat"`           // Legacy NAT
}

// APIConfig contains API server settings
type APIConfig struct {
	REST    RESTConfig    `yaml:"rest"`
	Metrics MetricsConfig `yaml:"metrics"`
}

// RESTConfig contains REST API settings
type RESTConfig struct {
	Enabled    bool     `yaml:"enabled"`
	Listen     string   `yaml:"listen"`
	CORS       bool     `yaml:"cors"`
	CORSOrigins []string `yaml:"cors_origins"` // Allowed CORS origins
	RateLimit  int      `yaml:"rate_limit"`   // Requests per second
	TLS        TLSConfig `yaml:"tls"`
	Auth       AuthConfig `yaml:"auth"`
}

// TLSConfig contains TLS settings
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	Enabled   bool   `yaml:"enabled"`
	JWTSecret string `yaml:"jwt_secret"`
	TokenTTL  string `yaml:"token_ttl"` // e.g., "24h", "7d"
}

// MetricsConfig contains metrics settings
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
	Path    string `yaml:"path"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // json, text
	Output string `yaml:"output"` // stdout, file
	File   string `yaml:"file"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Chain: ChainConfig{
			Name:    "polygon-pos",
			Network: "mainnet",
			ChainID: 137,
		},
		Checkpoint: CheckpointConfig{
			Source:       "mock", // Use "ethereum-l1" for production
			Contract:     "0x86E4Dc95c7FBdBf52e33D563BbDB00823894C287",
			Interval:     256,
			Timeout:      30 * time.Second,
			L1RPCURL:     "", // Add your Ethereum L1 RPC URL (Infura, Alchemy, etc.)
			L1ChainID:    1,  // 1 for Ethereum mainnet
			ValidatorRPC: "", // Optional: Heimdall API URL
		},
		Sync: SyncConfig{
			Forward: ForwardSyncConfig{
				Enabled:         true,
				Workers:         8,
				LookAhead:       10,
				ValidationLevel: "full", // full validation for forward sync
			},
			Backward: BackwardSyncConfig{
				Enabled:         true,
				Workers:         4,
				BatchSize:       10000,
				Throttle:        "50MB/s",
				ValidationLevel: "header", // light validation for backward sync
			},
		},
		Storage: StorageConfig{
			DataDir:         "/data/hypasis",
			CacheSize:       "2GB",
			WriteBufferSize: "256MB",
			MaxOpenFiles:    1024,
			HistoricalDepth: "full",
			Engine:          "pebble",
		},
		P2P: P2PConfig{
			Mode:          "rpc-server",
			RPCListen:     "0.0.0.0:8545",
			UpstreamRPCs:  []string{"https://polygon-rpc.com"},
			RPCTimeout:    30 * time.Second,
			RPCMaxRetries: 3,
			Listen:        "0.0.0.0:30303",
			MaxPeers:      50,
			NAT:           true,
		},
		API: APIConfig{
			REST: RESTConfig{
				Enabled:     true,
				Listen:      "0.0.0.0:8080",
				CORS:        true,
				CORSOrigins: []string{"*"},
				RateLimit:   100, // 100 requests per second
				TLS: TLSConfig{
					Enabled:  false, // Disabled by default
					CertFile: "/etc/hypasis/tls/cert.pem",
					KeyFile:  "/etc/hypasis/tls/key.pem",
				},
				Auth: AuthConfig{
					Enabled:   false, // Disabled by default
					JWTSecret: "change-this-secret-in-production",
					TokenTTL:  "24h",
				},
			},
			Metrics: MetricsConfig{
				Enabled: true,
				Listen:  "0.0.0.0:9090",
				Path:    "/metrics",
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
	}
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Chain.Name == "" {
		return fmt.Errorf("chain name is required")
	}

	if c.Sync.Forward.Workers < 1 {
		return fmt.Errorf("forward sync workers must be >= 1")
	}

	if c.Sync.Backward.Workers < 1 {
		return fmt.Errorf("backward sync workers must be >= 1")
	}

	if c.Storage.DataDir == "" {
		return fmt.Errorf("storage data directory is required")
	}

	return nil
}

// Save saves the configuration to a YAML file
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
