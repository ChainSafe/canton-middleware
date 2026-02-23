package main

import (
	"flag"
	"log"

	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/migrations/apidb"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/uptrace/bun/migrate"
)

func main() {
	cfgPath := flag.String("config", "config.example.yaml", "Path to configuration file")
	flag.Usage = mghelper.Usage
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadAPIServer(*cfgPath)
	if err != nil {
		log.Fatalf("error reading configuration file: %s", err.Error())
	}

	// Connect to database
	db, err := pgutil.ConnectDB(&cfg.Database)
	if err != nil {
		log.Fatalf("error connecting to database: %s", err.Error())
	}
	defer db.Close()

	log.Printf("Running migrations for API Server database (%s)...\n", cfg.Database.Database)

	// Create migrator
	migrator := migrate.NewMigrator(db, apidb.Migrations)

	// Run migrations with args
	err = mghelper.RunMigrations(migrator, flag.Args()...)
	if err != nil {
		mghelper.Exitf(err.Error())
	}
}
