package checkpoint

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/hypasis/sync-protocol/internal/types"
)

func TestSignatureVerifier_ECDSA(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	verifier := NewSignatureVerifier(big.NewInt(137))
	message := []byte("hypasis checkpoint message test")

	sig, err := verifier.CreateSignature(key, message)
	if err != nil {
		t.Fatalf("failed to create signature: %v", err)
	}

	if err := verifier.ValidateSignatureFormat(sig); err != nil {
		t.Errorf("signature format validation failed: %v", err)
	}

	expectedAddr := crypto.PubkeyToAddress(key.PublicKey)
	recoveredAddr, err := verifier.RecoverSignerAddress(message, sig)
	if err != nil {
		t.Fatalf("failed to recover signer address: %v", err)
	}

	if recoveredAddr != expectedAddr {
		t.Errorf("recovered address mismatch: got %s, want %s", recoveredAddr.Hex(), expectedAddr.Hex())
	}

	valid := verifier.VerifyECDSASignature(&key.PublicKey, message, sig)
	if !valid {
		t.Errorf("expected VerifyECDSASignature to return true, got false")
	}

	// Corrupt signature
	corruptSig := make([]byte, len(sig))
	copy(corruptSig, sig)
	corruptSig[0] ^= 0xff
	if verifier.VerifyECDSASignature(&key.PublicKey, message, corruptSig) {
		t.Errorf("expected VerifyECDSASignature to fail for corrupted signature")
	}
}

func TestSignatureVerifier_CheckpointSignatures(t *testing.T) {
	chainID := big.NewInt(137)
	verifier := NewSignatureVerifier(chainID)

	// Generate 3 validator keypairs
	valKeys := make([]*ecdsa.PrivateKey, 3)
	valAddrs := make([]common.Address, 3)
	validators := make([]*types.Validator, 3)

	stakes := []*big.Int{big.NewInt(400), big.NewInt(300), big.NewInt(300)} // Total stake = 1000

	for i := 0; i < 3; i++ {
		k, err := crypto.GenerateKey()
		if err != nil {
			t.Fatalf("failed to generate key %d: %v", i, err)
		}
		valKeys[i] = k
		valAddrs[i] = crypto.PubkeyToAddress(k.PublicKey)
		validators[i] = &types.Validator{
			ID:      valAddrs[i].Hex(),
			Address: valAddrs[i],
			Stake:   stakes[i],
			Active:  true,
		}
	}

	cp := &types.Checkpoint{
		BlockNumber: 100000,
		BlockHash:   common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
		StateRoot:   common.HexToHash("0xfedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"),
		ChainID:     chainID,
	}

	msg := verifier.createCheckpointMessage(cp)

	// Signatures for validator 0 (40% stake) and validator 1 (30% stake) -> 70% total stake (>= 66% threshold)
	sig0, err := verifier.CreateSignature(valKeys[0], msg)
	if err != nil {
		t.Fatalf("failed to sign cp for val 0: %v", err)
	}

	sig1, err := verifier.CreateSignature(valKeys[1], msg)
	if err != nil {
		t.Fatalf("failed to sign cp for val 1: %v", err)
	}

	cp.ValidatorSigs = []types.Signature{
		{ValidatorID: validators[0].ID, Signature: sig0},
		{ValidatorID: validators[1].ID, Signature: sig1},
	}

	// Should pass threshold (70% >= 66%)
	if err := verifier.VerifyCheckpointSignatures(cp, validators); err != nil {
		t.Errorf("expected checkpoint signature verification to pass, got: %v", err)
	}

	// Test insufficient stake: only validator 1 signs (30% < 66%)
	cpInsufficient := &types.Checkpoint{
		BlockNumber: 100000,
		BlockHash:   cp.BlockHash,
		StateRoot:   cp.StateRoot,
		ChainID:     chainID,
		ValidatorSigs: []types.Signature{
			{ValidatorID: validators[1].ID, Signature: sig1},
		},
	}

	if err := verifier.VerifyCheckpointSignatures(cpInsufficient, validators); err == nil {
		t.Errorf("expected failure due to insufficient stake, got nil error")
	}

	// Test corrupted signature for validator 0
	corruptSig0 := make([]byte, len(sig0))
	copy(corruptSig0, sig0)
	corruptSig0[10] ^= 0xff

	cpCorrupted := &types.Checkpoint{
		BlockNumber: 100000,
		BlockHash:   cp.BlockHash,
		StateRoot:   cp.StateRoot,
		ChainID:     chainID,
		ValidatorSigs: []types.Signature{
			{ValidatorID: validators[0].ID, Signature: corruptSig0},
			{ValidatorID: validators[1].ID, Signature: sig1},
		},
	}

	// Corrupted sig0 will be skipped, leaving only sig1 (30% stake) which fails threshold
	if err := verifier.VerifyCheckpointSignatures(cpCorrupted, validators); err == nil {
		t.Errorf("expected failure when signature is corrupted, got nil error")
	}
}
