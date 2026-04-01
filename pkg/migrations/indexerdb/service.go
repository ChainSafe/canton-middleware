// Package indexerdb holds all migrations for the indexer database.
package indexerdb

import (
	"github.com/uptrace/bun/migrate"
)

// Migrations is the collection of all migrations for the indexer database.
var Migrations = migrate.NewMigrations()
