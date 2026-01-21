package ethereum

import (
	"crypto/sha256"
	"encoding/binary"
)

// ComputeBlockHash generates a deterministic block hash from chain ID and block number.
// This is used for synthetic blocks in the Canton-EVM bridge.
func ComputeBlockHash(chainID, blockNumber uint64) []byte {
	data := make([]byte, 16)
	binary.BigEndian.PutUint64(data[0:8], chainID)
	binary.BigEndian.PutUint64(data[8:16], blockNumber)
	hash := sha256.Sum256(data)
	return hash[:]
}
