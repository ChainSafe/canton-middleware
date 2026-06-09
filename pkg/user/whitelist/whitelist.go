// SPDX-License-Identifier: Apache-2.0

// Package whitelist provides a shared address-authorization check used by both
// the registration service and the Ethereum JSON-RPC facade. The skip decision
// (skip_whitelist_check) is baked into the Checker at construction time, so
// consumers only ever ask "is this address allowed?" without carrying their own
// skip flag and branch.
package whitelist

import "context"

// Checker reports whether an EVM address is permitted. It is satisfied by the
// user store's IsWhitelisted method.
//
//go:generate mockery --name Checker --output mocks --outpkg mocks --filename mock_checker.go --with-expecter
type Checker interface {
	IsWhitelisted(ctx context.Context, evmAddress string) (bool, error)
}

// New returns a Checker. When skip is true every address is permitted;
// otherwise the decision delegates to src (typically the user store).
func New(src Checker, skip bool) Checker {
	if skip {
		return allowAll{}
	}
	return src
}

// allowAll authorizes every address. It is the value returned by New when the
// whitelist gate is disabled.
type allowAll struct{}

func (allowAll) IsWhitelisted(context.Context, string) (bool, error) { return true, nil }
