// Package apidb holds all the migrations for the API database
package apidb

import (
	"github.com/uptrace/bun/migrate"
)

// Migrations is the collection of all migrations for the API database
var Migrations = migrate.NewMigrations()
