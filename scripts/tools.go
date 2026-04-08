// Package tools pins script dependencies so go mod tidy does not remove them.
// All scripts in this directory use //go:build ignore, which causes go mod tidy
// to skip them when scanning imports. This file keeps those deps visible without
// pulling them into any binary (nothing imports this package).
package tools

import _ "github.com/lib/pq"
