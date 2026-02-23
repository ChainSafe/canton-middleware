// Package relayerdb holds all the migrations for the relayer database
package relayerdb

import (
	"github.com/uptrace/bun/migrate"
)

// Migrations is the collection of all migrations for the relayer database
var Migrations = migrate.NewMigrations()
