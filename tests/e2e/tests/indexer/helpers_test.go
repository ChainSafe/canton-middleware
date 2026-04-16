//go:build e2e

package indexer_test

import "math/big"

// amtGTE returns true when a >= b, comparing both as exact decimal strings.
// Uses big.Rat (exact rational arithmetic) to avoid the precision loss that
// big.Float can introduce with 18-decimal token amounts.
func amtGTE(a, b string) bool {
	af, ok1 := new(big.Rat).SetString(a)
	bf, ok2 := new(big.Rat).SetString(b)
	if !ok1 || !ok2 {
		return false
	}
	return af.Cmp(bf) >= 0
}

// amtLT returns true when a < b.
func amtLT(a, b string) bool {
	af, ok1 := new(big.Rat).SetString(a)
	bf, ok2 := new(big.Rat).SetString(b)
	if !ok1 || !ok2 {
		return false
	}
	return af.Cmp(bf) < 0
}
