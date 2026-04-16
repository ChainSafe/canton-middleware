//go:build e2e

package indexer_test

import "math/big"

// amtGTE returns true when a >= b, comparing both as decimal strings.
// Uses big.Float to handle 18-decimal token amounts correctly.
func amtGTE(a, b string) bool {
	af, ok1 := new(big.Float).SetString(a)
	bf, ok2 := new(big.Float).SetString(b)
	if !ok1 || !ok2 {
		return false
	}
	return af.Cmp(bf) >= 0
}

// amtLT returns true when a < b.
func amtLT(a, b string) bool {
	af, ok1 := new(big.Float).SetString(a)
	bf, ok2 := new(big.Float).SetString(b)
	if !ok1 || !ok2 {
		return false
	}
	return af.Cmp(bf) < 0
}
