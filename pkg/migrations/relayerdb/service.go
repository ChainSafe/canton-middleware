// Package relayerdb holds all the migrations for the relayer database
package relayerdb

import (
	"github.com/go-pg/migrations/v8"
)

// GetMigrations returns all migrations for the relayer database
func GetMigrations() []*migrations.Migration {
	migration := make([]*migrations.Migration, 0)
	migration = append(migration, createTransfers()...)
	migration = append(migration, createChainState()...)
	migration = append(migration, createNonceState()...)
	migration = append(migration, createBridgeBalances()...)
	return migration
}
