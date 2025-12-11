package validation

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/hypasis/sync-protocol/internal/types"
)

// Helper function to create a test block header
func makeTestHeader(number uint64, parentHash common.Hash) *types.Header {
	header := &types.Header{
		Number:      number,
		ParentHash:  parentHash,
		Timestamp:   uint64(time.Now().Unix()),
		GasLimit:    8000000,
		GasUsed:     0,
		Coinbase:    common.HexToAddress("0x0000000000000000000000000000000000000001"),
		StateRoot:   common.BytesToHash([]byte("state")),
		TxHash:      types.EmptyRootHash,
		ReceiptHash: types.EmptyRootHash,
		ExtraData:   make([]byte, 32), // Minimum vanity size
		MixDigest:   common.Hash{},
		Nonce:       0,
	}
	// Compute hash
	header.Hash = computeTestHeaderHash(header)
	return header
}

// Helper to compute header hash for tests (matches validator logic)
func computeTestHeaderHash(header *types.Header) common.Hash {
	// Use the same hash computation as the validator
	hasher := crypto.NewKeccakState()
	rlp.Encode(hasher, []interface{}{
		header.ParentHash,
		header.Coinbase,
		header.StateRoot,
		header.TxHash,
		header.ReceiptHash,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Timestamp,
		header.ExtraData,
		header.MixDigest,
		header.Nonce,
	})
	var hash common.Hash
	hasher.Read(hash[:])
	return hash
}

// Helper function to create a test block
func makeTestBlock(number uint64, parentHash common.Hash) *types.Block {
	header := makeTestHeader(number, parentHash)
	return &types.Block{
		Header:       header,
		Transactions: []*types.Transaction{},
	}
}

// TestPolygonValidator_ValidateBlockHeader tests header validation
func TestPolygonValidator_ValidateBlockHeader(t *testing.T) {
	validator := NewPolygonValidator(DefaultPolygonConfig())
	ctx := context.Background()

	// Create a valid parent and child
	parent := makeTestHeader(100, common.Hash{})
	child := makeTestHeader(101, parent.Hash)
	child.Timestamp = parent.Timestamp + 2 // 2 seconds later
	child.Hash = computeTestHeaderHash(child) // Recompute hash after timestamp change

	// Test valid header
	err := validator.ValidateBlockHeader(ctx, child, parent)
	if err != nil {
		t.Errorf("Valid header failed validation: %v", err)
	}
}

// TestPolygonValidator_InvalidBlockNumber tests invalid block number
func TestPolygonValidator_InvalidBlockNumber(t *testing.T) {
	validator := NewPolygonValidator(DefaultPolygonConfig())
	ctx := context.Background()

	parent := makeTestHeader(100, common.Hash{})
	child := makeTestHeader(102, parent.Hash) // Skip a number

	err := validator.ValidateBlockHeader(ctx, child, parent)
	if err == nil {
		t.Error("Expected error for invalid block number, got nil")
	}
}

// TestPolygonValidator_InvalidParentHash tests invalid parent hash
func TestPolygonValidator_InvalidParentHash(t *testing.T) {
	validator := NewPolygonValidator(DefaultPolygonConfig())
	ctx := context.Background()

	parent := makeTestHeader(100, common.Hash{})
	child := makeTestHeader(101, common.BytesToHash([]byte("wrong")))

	err := validator.ValidateBlockHeader(ctx, child, parent)
	if err == nil {
		t.Error("Expected error for invalid parent hash, got nil")
	}
}

// TestPolygonValidator_InvalidTimestamp tests invalid timestamp
func TestPolygonValidator_InvalidTimestamp(t *testing.T) {
	validator := NewPolygonValidator(DefaultPolygonConfig())
	ctx := context.Background()

	parent := makeTestHeader(100, common.Hash{})
	child := makeTestHeader(101, parent.Hash)
	child.Timestamp = parent.Timestamp - 1 // Timestamp in the past
	child.Hash = computeTestHeaderHash(child) // Recompute hash after timestamp change

	err := validator.ValidateBlockHeader(ctx, child, parent)
	if err == nil {
		t.Error("Expected error for invalid timestamp, got nil")
	}
}

// TestPolygonValidator_InvalidGasLimit tests invalid gas limit
func TestPolygonValidator_InvalidGasLimit(t *testing.T) {
	validator := NewPolygonValidator(DefaultPolygonConfig())
	ctx := context.Background()

	tests := []struct {
		name     string
		gasLimit uint64
		wantErr  bool
	}{
		{"Too low", 1000, true},
		{"Valid", 8000000, false},
		{"Too high", 50000000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := makeTestHeader(100, common.Hash{})
			header.GasLimit = tt.gasLimit

			err := validator.ValidateBlockHeader(ctx, header, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("gasLimit=%d: wantErr=%v, got err=%v", tt.gasLimit, tt.wantErr, err)
			}
		})
	}
}

// TestPolygonValidator_GasUsedExceedsLimit tests gas used exceeding limit
func TestPolygonValidator_GasUsedExceedsLimit(t *testing.T) {
	validator := NewPolygonValidator(DefaultPolygonConfig())
	ctx := context.Background()

	header := makeTestHeader(100, common.Hash{})
	header.GasLimit = 8000000
	header.GasUsed = 9000000 // Exceeds limit

	err := validator.ValidateBlockHeader(ctx, header, nil)
	if err == nil {
		t.Error("Expected error for gas used exceeding limit, got nil")
	}
}

// TestPolygonValidator_GasLimitChange tests gas limit change validation
func TestPolygonValidator_GasLimitChange(t *testing.T) {
	validator := NewPolygonValidator(DefaultPolygonConfig())
	ctx := context.Background()

	parent := makeTestHeader(100, common.Hash{})
	parent.GasLimit = 8000000

	tests := []struct {
		name     string
		gasLimit uint64
		wantErr  bool
	}{
		{"Small increase", 8007812, false},  // Within 1/1024 limit
		{"Small decrease", 7992188, false},  // Within 1/1024 limit
		{"Large increase", 9000000, true},   // Exceeds 1/1024 limit
		{"Large decrease", 7000000, true},   // Exceeds 1/1024 limit
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			child := makeTestHeader(101, parent.Hash)
			child.GasLimit = tt.gasLimit
			child.Timestamp = parent.Timestamp + 2
			// Recompute hash after modifying fields
			child.Hash = computeTestHeaderHash(child)

			err := validator.ValidateBlockHeader(ctx, child, parent)
			if (err != nil) != tt.wantErr {
				t.Errorf("gasLimit=%d: wantErr=%v, got err=%v", tt.gasLimit, tt.wantErr, err)
			}
		})
	}
}

// TestPolygonValidator_ValidateTransaction tests transaction validation
func TestPolygonValidator_ValidateTransaction(t *testing.T) {
	validator := NewPolygonValidator(DefaultPolygonConfig())

	// Create a valid transaction
	tx := &types.Transaction{
		Nonce:    1,
		GasPrice: big.NewInt(1000000000), // 1 gwei
		Gas:      21000,
		To:       common.HexToAddress("0x1234567890123456789012345678901234567890"),
		Value:    big.NewInt(1000000000000000000), // 1 ETH
		Data:     []byte{},
		V:        big.NewInt(27),
		R:        big.NewInt(1),
		S:        big.NewInt(1),
	}
	// Compute hash
	tx.Hash = computeTestTxHash(tx)

	err := validator.validateTransaction(tx, 100)
	if err != nil {
		t.Errorf("Valid transaction failed validation: %v", err)
	}
}

// Helper to compute tx hash for tests (matches validator logic)
func computeTestTxHash(tx *types.Transaction) common.Hash {
	// Use the same hash computation as the validator
	hasher := crypto.NewKeccakState()
	rlp.Encode(hasher, []interface{}{
		tx.Nonce,
		tx.GasPrice,
		tx.Gas,
		tx.To,
		tx.Value,
		tx.Data,
		tx.V,
		tx.R,
		tx.S,
	})
	var hash common.Hash
	hasher.Read(hash[:])
	return hash
}

// TestPolygonValidator_InvalidTransaction tests invalid transactions
func TestPolygonValidator_InvalidTransaction(t *testing.T) {
	validator := NewPolygonValidator(DefaultPolygonConfig())

	tests := []struct {
		name    string
		modifyTx func(*types.Transaction)
		wantErr bool
	}{
		{
			name: "Zero gas",
			modifyTx: func(tx *types.Transaction) {
				tx.Gas = 0
			},
			wantErr: true,
		},
		{
			name: "Negative gas price",
			modifyTx: func(tx *types.Transaction) {
				tx.GasPrice = big.NewInt(-1)
			},
			wantErr: true,
		},
		{
			name: "Negative value",
			modifyTx: func(tx *types.Transaction) {
				tx.Value = big.NewInt(-1)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &types.Transaction{
				Nonce:    1,
				GasPrice: big.NewInt(1000000000),
				Gas:      21000,
				To:       common.HexToAddress("0x1234567890123456789012345678901234567890"),
				Value:    big.NewInt(1000000000000000000),
				Data:     []byte{},
				V:        big.NewInt(27),
				R:        big.NewInt(1),
				S:        big.NewInt(1),
			}
			tx.Hash = computeTestTxHash(tx)

			tt.modifyTx(tx)

			err := validator.validateTransaction(tx, 100)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

// TestPolygonValidator_ValidateExtraData tests extra data validation
func TestPolygonValidator_ValidateExtraData(t *testing.T) {
	validator := NewPolygonValidator(DefaultPolygonConfig())

	tests := []struct {
		name         string
		extraData    []byte
		isSprintEnd  bool
		wantErr      bool
	}{
		{
			name:        "Valid vanity",
			extraData:   make([]byte, 32),
			isSprintEnd: false,
			wantErr:     false,
		},
		{
			name:        "Valid sprint block",
			extraData:   make([]byte, 97), // 32 vanity + 65 signature
			isSprintEnd: true,
			wantErr:     false,
		},
		{
			name:        "Too short",
			extraData:   make([]byte, 10),
			isSprintEnd: false,
			wantErr:     true,
		},
		{
			name:        "Sprint block too short",
			extraData:   make([]byte, 50),
			isSprintEnd: true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := makeTestHeader(100, common.Hash{})
			header.ExtraData = tt.extraData

			err := validator.validateExtraData(header, tt.isSprintEnd)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

// TestPolygonValidator_ValidateFullBlock tests full block validation
func TestPolygonValidator_ValidateFullBlock(t *testing.T) {
	validator := NewPolygonValidator(DefaultPolygonConfig())
	ctx := context.Background()

	parent := makeTestBlock(100, common.Hash{})
	child := makeTestBlock(101, parent.Header.Hash)
	child.Header.Timestamp = parent.Header.Timestamp + 2
	child.Header.Hash = computeTestHeaderHash(child.Header) // Recompute hash after timestamp change

	err := validator.ValidateBlock(ctx, child, parent)
	if err != nil {
		t.Errorf("Valid block failed validation: %v", err)
	}
}

// TestLightValidator tests light validator (header only)
func TestLightValidator(t *testing.T) {
	validator := NewLightValidator(DefaultPolygonConfig())
	ctx := context.Background()

	parent := makeTestBlock(100, common.Hash{})
	child := makeTestBlock(101, parent.Header.Hash)
	child.Header.Timestamp = parent.Header.Timestamp + 2
	child.Header.Hash = computeTestHeaderHash(child.Header) // Recompute hash after timestamp change

	// Light validator should only validate header
	err := validator.ValidateBlock(ctx, child, parent)
	if err != nil {
		t.Errorf("Valid block failed light validation: %v", err)
	}
}

// TestNoOpValidator tests no-op validator
func TestNoOpValidator(t *testing.T) {
	validator := NewNoOpValidator()
	ctx := context.Background()

	// Create completely invalid block
	block := &types.Block{
		Header: &types.Header{
			Number:   0, // Invalid
			GasLimit: 0, // Invalid
		},
	}

	// NoOp validator should pass everything
	err := validator.ValidateBlock(ctx, block, nil)
	if err != nil {
		t.Errorf("NoOp validator should not return errors, got: %v", err)
	}
}

// TestBorConsensusRules tests Bor consensus rules
func TestBorConsensusRules(t *testing.T) {
	rules := NewBorConsensusRules(PolygonMainnetConfig)

	tests := []struct {
		name        string
		blockNumber uint64
		wantSprintEnd bool
	}{
		{"Sprint end", 16, true},
		{"Sprint end", 32, true},
		{"Not sprint end", 15, false},
		{"Not sprint end", 17, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rules.IsSprintEnd(tt.blockNumber)
			if got != tt.wantSprintEnd {
				t.Errorf("IsSprintEnd(%d) = %v, want %v", tt.blockNumber, got, tt.wantSprintEnd)
			}
		})
	}
}

// TestValidationLevel tests validation level parsing
func TestValidationLevel(t *testing.T) {
	tests := []struct {
		input string
		want  ValidationLevel
	}{
		{"none", ValidationNone},
		{"header", ValidationHeader},
		{"light", ValidationLight},
		{"full", ValidationFull},
		{"invalid", ValidationFull}, // Default to full
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseValidationLevel(tt.input)
			if got != tt.want {
				t.Errorf("ParseValidationLevel(%s) = %v, want %v", tt.input, got, tt.want)
			}

			// Test String() method
			if tt.want != ValidationFull || tt.input != "invalid" {
				str := got.String()
				if str != tt.input {
					t.Errorf("ValidationLevel.String() = %s, want %s", str, tt.input)
				}
			}
		})
	}
}

// TestGetChainConfig tests chain config retrieval
func TestGetChainConfig(t *testing.T) {
	tests := []struct {
		chainID *big.Int
		want    string
	}{
		{big.NewInt(137), "polygon-mainnet"},
		{big.NewInt(80002), "polygon-amoy"},
		{big.NewInt(80001), "polygon-mumbai"},
		{big.NewInt(999), "polygon-mainnet"}, // Unknown defaults to mainnet
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			cfg := GetChainConfig(tt.chainID)
			if cfg.ChainName != tt.want {
				t.Errorf("GetChainConfig(%d).ChainName = %s, want %s", tt.chainID, cfg.ChainName, tt.want)
			}
		})
	}
}

// BenchmarkValidateBlockHeader benchmarks header validation
func BenchmarkValidateBlockHeader(b *testing.B) {
	validator := NewPolygonValidator(DefaultPolygonConfig())
	ctx := context.Background()

	parent := makeTestHeader(100, common.Hash{})
	child := makeTestHeader(101, parent.Hash)
	child.Timestamp = parent.Timestamp + 2
	child.Hash = computeTestHeaderHash(child) // Recompute hash after timestamp change

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateBlockHeader(ctx, child, parent)
	}
}

// BenchmarkValidateBlock benchmarks full block validation
func BenchmarkValidateBlock(b *testing.B) {
	validator := NewPolygonValidator(DefaultPolygonConfig())
	ctx := context.Background()

	parent := makeTestBlock(100, common.Hash{})
	child := makeTestBlock(101, parent.Header.Hash)
	child.Header.Timestamp = parent.Header.Timestamp + 2
	child.Header.Hash = computeTestHeaderHash(child.Header) // Recompute hash after timestamp change

	// Add some transactions
	for i := 0; i < 10; i++ {
		tx := &types.Transaction{
			Nonce:    uint64(i),
			GasPrice: big.NewInt(1000000000),
			Gas:      21000,
			To:       common.HexToAddress("0x1234567890123456789012345678901234567890"),
			Value:    big.NewInt(1000000000000000000),
			Data:     []byte{},
			V:        big.NewInt(27),
			R:        big.NewInt(1),
			S:        big.NewInt(1),
		}
		tx.Hash = computeTestTxHash(tx)
		child.Transactions = append(child.Transactions, tx)
	}

	// Recompute tx hash root
	child.Header.TxHash = validator.computeTxHashRoot(child.Transactions)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateBlock(ctx, child, parent)
	}
}
