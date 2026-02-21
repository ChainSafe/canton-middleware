// Package apidb holds all the migrations for the API database
package apidb

import (
	"log"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
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

// Migrate runs all migrations for the API database
func Migrate(db *pg.DB) error {
	coll := migrations.NewCollection(GetMigrations()...).DisableSQLAutodiscover(true)

	_, _, err := coll.Run(db, "init")
	if err != nil {
		log.Printf("API DB init: %v", err)
	}

	oldVersion, newVersion, err := coll.Run(db, "up")
	if err != nil {
		return err
	}

	if newVersion != oldVersion {
		log.Printf("API DB migrated from version %d to %d\n", oldVersion, newVersion)
	} else {
		log.Printf("API DB version is %d\n", oldVersion)
	}

	return nil
}
