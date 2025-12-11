package checkpoint

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/hypasis/sync-protocol/internal/types"
)

// SignatureVerifier handles cryptographic signature verification
type SignatureVerifier struct {
	chainID *big.Int
}

// NewSignatureVerifier creates a new signature verifier
func NewSignatureVerifier(chainID *big.Int) *SignatureVerifier {
	return &SignatureVerifier{
		chainID: chainID,
	}
}

// VerifyECDSASignature verifies an ECDSA signature
func (v *SignatureVerifier) VerifyECDSASignature(
	publicKey *ecdsa.PublicKey,
	message []byte,
	signature []byte,
) bool {
	// Hash the message using Keccak256
	messageHash := crypto.Keccak256Hash(message)

	// Verify the signature
	// Ethereum signatures are [R || S || V] format (65 bytes)
	if len(signature) != 65 {
		return false
	}

	// Extract R, S, V components
	// V is the recovery id (last byte)
	// Subtract 27 from V if needed (Ethereum convention)
	v_byte := signature[64]
	if v_byte >= 27 {
		v_byte -= 27
	}

	// Recover the public key from signature
	recoveredPub, err := crypto.SigToPub(messageHash.Bytes(), signature)
	if err != nil {
		return false
	}

	// Compare recovered public key with expected public key
	recoveredAddr := crypto.PubkeyToAddress(*recoveredPub)
	expectedAddr := crypto.PubkeyToAddress(*publicKey)

	return recoveredAddr == expectedAddr
}

// VerifyCheckpointSignature verifies a checkpoint signature against a validator
func (v *SignatureVerifier) VerifyCheckpointSignature(
	checkpoint *types.Checkpoint,
	validator *types.Validator,
	signature []byte,
) error {
	// Create the message to be signed
	// In Polygon, this is typically:
	// keccak256(abi.encodePacked(blockNumber, blockHash, stateRoot))
	message := v.createCheckpointMessage(checkpoint)

	// For ECDSA verification, we need the validator's public key
	// In production, this would be derived from the validator's address
	// For now, we'll use a simplified verification

	// Hash the message
	messageHash := crypto.Keccak256Hash(message)

	// Verify signature length
	if len(signature) != 65 {
		return fmt.Errorf("invalid signature length: expected 65, got %d", len(signature))
	}

	// Recover the signer's address from the signature
	recoveredPub, err := crypto.SigToPub(messageHash.Bytes(), signature)
	if err != nil {
		return fmt.Errorf("failed to recover public key: %w", err)
	}

	recoveredAddr := crypto.PubkeyToAddress(*recoveredPub)

	// Verify the recovered address matches the validator's address
	if recoveredAddr != validator.Address {
		return fmt.Errorf("signature verification failed: recovered address %s does not match validator address %s",
			recoveredAddr.Hex(), validator.Address.Hex())
	}

	return nil
}

// createCheckpointMessage creates the message that validators sign
func (v *SignatureVerifier) createCheckpointMessage(checkpoint *types.Checkpoint) []byte {
	// Polygon checkpoint message format:
	// keccak256(abi.encodePacked(blockNumber, blockHash, stateRoot))

	// Convert block number to 32 bytes (big-endian)
	blockNumBytes := make([]byte, 32)
	blockNum := big.NewInt(int64(checkpoint.BlockNumber))
	blockNum.FillBytes(blockNumBytes)

	// Concatenate: blockNumber (32 bytes) + blockHash (32 bytes) + stateRoot (32 bytes)
	message := make([]byte, 0, 96)
	message = append(message, blockNumBytes...)
	message = append(message, checkpoint.BlockHash.Bytes()...)
	message = append(message, checkpoint.StateRoot.Bytes()...)

	return message
}

// VerifyCheckpointSignatures verifies all signatures on a checkpoint
func (v *SignatureVerifier) VerifyCheckpointSignatures(
	checkpoint *types.Checkpoint,
	validators []*types.Validator,
) error {
	if len(checkpoint.ValidatorSigs) == 0 {
		return fmt.Errorf("no signatures on checkpoint")
	}

	// Create a map of validator ID to validator
	validatorMap := make(map[string]*types.Validator)
	for _, val := range validators {
		validatorMap[val.ID] = val
	}

	// Verify each signature
	validSignatures := 0
	totalStake := big.NewInt(0)
	signedStake := big.NewInt(0)

	for _, sig := range checkpoint.ValidatorSigs {
		validator, ok := validatorMap[sig.ValidatorID]
		if !ok {
			// Unknown validator, skip
			continue
		}

		totalStake.Add(totalStake, validator.Stake)

		// Verify the signature
		err := v.VerifyCheckpointSignature(checkpoint, validator, sig.Signature)
		if err != nil {
			// Invalid signature, skip
			continue
		}

		validSignatures++
		signedStake.Add(signedStake, validator.Stake)
	}

	// Check if we have enough signatures (2/3+ of total stake)
	threshold := new(big.Int).Mul(totalStake, big.NewInt(66))
	threshold.Div(threshold, big.NewInt(100))

	if signedStake.Cmp(threshold) < 0 {
		return fmt.Errorf("insufficient stake signed: got %s, need %s (66%% of %s)",
			signedStake.String(), threshold.String(), totalStake.String())
	}

	return nil
}

// RecoverSignerAddress recovers the address that signed a message
func (v *SignatureVerifier) RecoverSignerAddress(message []byte, signature []byte) (common.Address, error) {
	if len(signature) != 65 {
		return common.Address{}, fmt.Errorf("invalid signature length")
	}

	// Hash the message
	messageHash := crypto.Keccak256Hash(message)

	// Recover the public key
	recoveredPub, err := crypto.SigToPub(messageHash.Bytes(), signature)
	if err != nil {
		return common.Address{}, err
	}

	// Convert to address
	return crypto.PubkeyToAddress(*recoveredPub), nil
}

// VerifyBLSSignature verifies a BLS signature (for future use)
// BLS signatures are more efficient for aggregation
func (v *SignatureVerifier) VerifyBLSSignature(
	publicKey []byte,
	message []byte,
	signature []byte,
) (bool, error) {
	// BLS signature verification would go here
	// Requires: github.com/herumi/bls-eth-go-binary
	/*
		import bls "github.com/herumi/bls-eth-go-binary/bls"

		// Initialize BLS
		bls.Init(bls.BLS12_381)
		bls.SetETHmode(bls.EthModeLatest)

		// Parse public key
		var pub bls.PublicKey
		if err := pub.Deserialize(publicKey); err != nil {
			return false, err
		}

		// Parse signature
		var sig bls.Sign
		if err := sig.Deserialize(signature); err != nil {
			return false, err
		}

		// Verify
		return sig.VerifyByte(&pub, message), nil
	*/

	return false, fmt.Errorf("BLS signature verification not yet implemented")
}

// AggregateBLSSignatures aggregates multiple BLS signatures (for future use)
func (v *SignatureVerifier) AggregateBLSSignatures(signatures [][]byte) ([]byte, error) {
	// BLS signature aggregation would go here
	// This allows combining multiple signatures into one
	return nil, fmt.Errorf("BLS aggregation not yet implemented")
}

// CreateSignature creates a signature for testing purposes
func (v *SignatureVerifier) CreateSignature(privateKey *ecdsa.PrivateKey, message []byte) ([]byte, error) {
	// Hash the message
	messageHash := crypto.Keccak256Hash(message)

	// Sign the hash
	signature, err := crypto.Sign(messageHash.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}

	return signature, nil
}

// ValidateSignatureFormat validates that a signature is properly formatted
func (v *SignatureVerifier) ValidateSignatureFormat(signature []byte) error {
	if len(signature) != 65 {
		return fmt.Errorf("invalid ECDSA signature length: expected 65 bytes, got %d", len(signature))
	}

	// Check recovery ID (last byte) is valid (0, 1, 27, or 28)
	recoveryID := signature[64]
	if recoveryID != 0 && recoveryID != 1 && recoveryID != 27 && recoveryID != 28 {
		return fmt.Errorf("invalid recovery ID: %d", recoveryID)
	}

	return nil
}

// GetMessageHash returns the hash of a message as it would be signed
func (v *SignatureVerifier) GetMessageHash(message []byte) common.Hash {
	return crypto.Keccak256Hash(message)
}
