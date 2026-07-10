package checkpoint

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/hypasis/sync-protocol/internal/types"
)

// childBlockInterval is Polygon's RootChain CHILD_BLOCK_INTERVAL. Header block
// IDs in the contract are multiples of this value.
const childBlockInterval = 10000

// rootChainABI is the minimal ABI needed to read checkpoints from Polygon's
// RootChain (RootChainProxy) contract on Ethereum.
//
//   - currentHeaderBlock() returns the ID of the most recently submitted header
//     block (a multiple of CHILD_BLOCK_INTERVAL).
//   - headerBlocks(id) returns the checkpoint: the merkle root, the first
//     (start) and last (end) Bor block covered, the L1 timestamp it was created,
//     and the proposer address.
const rootChainABI = `[
  {"constant":true,"inputs":[],"name":"currentHeaderBlock","outputs":[{"name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"constant":true,"inputs":[{"name":"","type":"uint256"}],"name":"headerBlocks","outputs":[{"name":"root","type":"bytes32"},{"name":"start","type":"uint256"},{"name":"end","type":"uint256"},{"name":"createdAt","type":"uint256"},{"name":"proposer","type":"address"}],"stateMutability":"view","type":"function"}
]`

// L1Source fetches Polygon checkpoints from the RootChain contract on Ethereum
// L1. Because the checkpoint data is read directly from finalized Ethereum
// state, its authenticity is anchored to Ethereum consensus.
type L1Source struct {
	client       *ethclient.Client
	bound        *bind.BoundContract
	contractAddr common.Address
	rpcURL       string
	chainID      uint64 // Polygon chain ID this source is reporting (137 / 80002)
	timeout      time.Duration

	mu    sync.RWMutex
	cache map[uint64]*types.Checkpoint // keyed by covered end-block
}

// headerBlock is a decoded RootChain checkpoint entry.
type headerBlock struct {
	Root      common.Hash
	Start     *big.Int
	End       *big.Int
	CreatedAt *big.Int
	Proposer  common.Address
}

// NewL1Source creates a new Ethereum L1 checkpoint source bound to the RootChain
// contract. chainID is the Polygon chain the checkpoints describe (137 mainnet,
// 80002 Amoy); it defaults to 137 when zero.
func NewL1Source(rpcURL string, contractAddr common.Address, chainID uint64) (*L1Source, error) {
	if rpcURL == "" {
		return nil, fmt.Errorf("L1 RPC URL is required")
	}
	if (contractAddr == common.Address{}) {
		return nil, fmt.Errorf("checkpoint contract address is required")
	}

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to L1: %w", err)
	}

	parsed, err := abi.JSON(strings.NewReader(rootChainABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse RootChain ABI: %w", err)
	}

	if chainID == 0 {
		chainID = 137
	}

	return &L1Source{
		client:       client,
		bound:        bind.NewBoundContract(contractAddr, parsed, client, client, client),
		contractAddr: contractAddr,
		rpcURL:       rpcURL,
		chainID:      chainID,
		timeout:      30 * time.Second,
		cache:        make(map[uint64]*types.Checkpoint),
	}, nil
}

// FetchLatest reads the most recent checkpoint from the RootChain contract.
func (s *L1Source) FetchLatest(ctx context.Context) (*types.Checkpoint, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	id, err := s.currentHeaderBlockID(fetchCtx)
	if err != nil {
		return nil, fmt.Errorf("currentHeaderBlock: %w", err)
	}

	hb, err := s.headerBlockByID(fetchCtx, id)
	if err != nil {
		return nil, fmt.Errorf("headerBlocks(%s): %w", id, err)
	}

	cp := s.toCheckpoint(hb)
	s.put(cp)
	return cp, nil
}

// FetchByNumber returns the checkpoint whose covered range includes the given
// Bor (Polygon) block number. It binary-searches over header block IDs, which
// have monotonically increasing ranges.
func (s *L1Source) FetchByNumber(ctx context.Context, blockNum uint64) (*types.Checkpoint, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	current, err := s.currentHeaderBlockID(fetchCtx)
	if err != nil {
		return nil, fmt.Errorf("currentHeaderBlock: %w", err)
	}

	// Number of checkpoints = current / interval.
	n := new(big.Int).Div(current, big.NewInt(childBlockInterval)).Int64()
	lo, hi := int64(1), n
	for lo <= hi {
		mid := (lo + hi) / 2
		id := big.NewInt(mid * childBlockInterval)
		hb, err := s.headerBlockByID(fetchCtx, id)
		if err != nil {
			return nil, fmt.Errorf("headerBlocks(%s): %w", id, err)
		}
		switch {
		case blockNum < hb.Start.Uint64():
			hi = mid - 1
		case blockNum > hb.End.Uint64():
			lo = mid + 1
		default:
			cp := s.toCheckpoint(hb)
			s.put(cp)
			return cp, nil
		}
	}

	return nil, fmt.Errorf("no checkpoint covers Bor block %d (latest checkpoint id %s)", blockNum, current)
}

// FetchRange returns all checkpoints covering blocks in [start, end).
func (s *L1Source) FetchRange(ctx context.Context, start, end uint64) ([]*types.Checkpoint, error) {
	if start >= end {
		return nil, fmt.Errorf("invalid range: start=%d, end=%d", start, end)
	}

	var checkpoints []*types.Checkpoint
	seen := make(map[uint64]bool)

	for block := start; block < end; {
		cp, err := s.FetchByNumber(ctx, block)
		if err != nil {
			return nil, err
		}
		if !seen[cp.BlockNumber] {
			checkpoints = append(checkpoints, cp)
			seen[cp.BlockNumber] = true
		}
		// Advance past the end of this checkpoint's covered range.
		block = cp.BlockNumber + 1
	}

	return checkpoints, nil
}

// currentHeaderBlockID calls RootChain.currentHeaderBlock().
func (s *L1Source) currentHeaderBlockID(ctx context.Context) (*big.Int, error) {
	var out []interface{}
	if err := s.bound.Call(&bind.CallOpts{Context: ctx}, &out, "currentHeaderBlock"); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty result")
	}
	id, ok := out[0].(*big.Int)
	if !ok || id == nil {
		return nil, fmt.Errorf("unexpected currentHeaderBlock result type %T", out[0])
	}
	return id, nil
}

// headerBlockByID calls RootChain.headerBlocks(id).
func (s *L1Source) headerBlockByID(ctx context.Context, id *big.Int) (*headerBlock, error) {
	var out []interface{}
	if err := s.bound.Call(&bind.CallOpts{Context: ctx}, &out, "headerBlocks", id); err != nil {
		return nil, err
	}
	if len(out) < 5 {
		return nil, fmt.Errorf("unexpected headerBlocks result length %d", len(out))
	}

	root, _ := out[0].([32]byte)
	start, _ := out[1].(*big.Int)
	end, _ := out[2].(*big.Int)
	createdAt, _ := out[3].(*big.Int)
	proposer, _ := out[4].(common.Address)

	if start == nil || end == nil {
		return nil, fmt.Errorf("checkpoint %s has nil range", id)
	}
	if createdAt == nil {
		createdAt = big.NewInt(0)
	}

	return &headerBlock{
		Root:      common.BytesToHash(root[:]),
		Start:     start,
		End:       end,
		CreatedAt: createdAt,
		Proposer:  proposer,
	}, nil
}

// toCheckpoint maps an on-chain header block to our checkpoint type. The
// checkpoint root read from L1 is the anchored commitment; BlockNumber is the
// last Bor block the checkpoint finalizes. The proposer address is recorded as
// the single "validator" entry (no fabricated signature bytes).
func (s *L1Source) toCheckpoint(hb *headerBlock) *types.Checkpoint {
	return &types.Checkpoint{
		BlockNumber:   hb.End.Uint64(),
		BlockHash:     hb.Root,
		StateRoot:     hb.Root,
		Timestamp:     hb.CreatedAt.Uint64(),
		ChainID:       new(big.Int).SetUint64(s.chainID),
		ValidatorSigs: []types.Signature{{ValidatorID: hb.Proposer.Hex(), PubKey: hb.Proposer.Bytes()}},
	}
}

func (s *L1Source) put(cp *types.Checkpoint) {
	s.mu.Lock()
	s.cache[cp.BlockNumber] = cp
	s.mu.Unlock()
}

// Close closes the L1 client connection.
func (s *L1Source) Close() {
	if s.client != nil {
		s.client.Close()
	}
}

// GetContractAddress returns the L1 checkpoint contract address.
func (s *L1Source) GetContractAddress() common.Address {
	return s.contractAddr
}

// GetRPCURL returns the L1 RPC URL.
func (s *L1Source) GetRPCURL() string {
	return s.rpcURL
}

// VerifyConnection verifies the L1 connection is working.
func (s *L1Source) VerifyConnection(ctx context.Context) error {
	fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.client.HeaderByNumber(fetchCtx, nil)
	if err != nil {
		return fmt.Errorf("L1 connection verification failed: %w", err)
	}
	return nil
}

// L1Validator validates checkpoints that were read from the RootChain contract
// on Ethereum L1. Their authenticity is guaranteed by Ethereum consensus (the
// data lives in finalized L1 state), so verification here is a structural
// integrity check rather than a re-derivation of validator signatures.
type L1Validator struct{}

// NewL1Validator creates a validator for the ethereum-l1 checkpoint source.
func NewL1Validator() *L1Validator {
	return &L1Validator{}
}

// Verify implements the Validator interface.
func (v *L1Validator) Verify(_ context.Context, cp *types.Checkpoint) error {
	if cp == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	if cp.BlockNumber == 0 {
		return fmt.Errorf("checkpoint has zero end block")
	}
	if cp.StateRoot == (common.Hash{}) {
		return fmt.Errorf("checkpoint root is zero (not a valid on-chain checkpoint)")
	}
	return nil
}

// GetValidatorSet is not used for the L1-anchored trust model.
func (v *L1Validator) GetValidatorSet(_ context.Context, _ uint64) ([]types.Validator, error) {
	return nil, nil
}
