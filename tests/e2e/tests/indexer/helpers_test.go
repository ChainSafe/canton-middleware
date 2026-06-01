//go:build e2e

package indexer_test

import "github.com/chainsafe/canton-middleware/tests/e2e/devstack/dsl"

// amtGTE and amtLT delegate to the shared DSL helpers to avoid duplicating
// the big.Rat comparison logic here.

func amtGTE(a, b string) bool { return dsl.AmountGTE(a, b) }
func amtLT(a, b string) bool  { return dsl.AmountLT(a, b) }
