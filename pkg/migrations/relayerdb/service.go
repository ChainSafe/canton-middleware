// Package relayerdb holds all the migrations for the relayer database
package relayerdb

import (
	"log"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
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

// Migrate runs all migrations for the relayer database
func Migrate(db *pg.DB) error {
	coll := migrations.NewCollection(GetMigrations()...).DisableSQLAutodiscover(true)

	_, _, err := coll.Run(db, "init")
	if err != nil {
		log.Printf("Relayer DB init: %v", err)
	}

	oldVersion, newVersion, err := coll.Run(db, "up")
	if err != nil {
		return err
	}

	if newVersion != oldVersion {
		log.Printf("Relayer DB migrated from version %d to %d\n", oldVersion, newVersion)
	} else {
		log.Printf("Relayer DB version is %d\n", oldVersion)
	}

	return nil
}
