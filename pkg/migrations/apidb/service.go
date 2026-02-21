// Package apidb holds all the migrations for the API database
package apidb

import (
	"github.com/go-pg/migrations/v8"
)

// GetMigrations returns all migrations for the API database
func GetMigrations() []*migrations.Migration {
	migration := make([]*migrations.Migration, 0)
	migration = append(migration, createUsers()...)
	migration = append(migration, createWhitelist()...)
	migration = append(migration, createTokenMetrics()...)
	migration = append(migration, seedTokenMetrics()...)
	migration = append(migration, createBridgeEvents()...)
	migration = append(migration, createReconciliationState()...)
	migration = append(migration, createEvmTransactions()...)
	migration = append(migration, createEvmMeta()...)
	migration = append(migration, createEvmLogs()...)
	return migration
}
