package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/migrations/relayerdb"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func main() {
	cfgPath := flag.String("config", "config.example.yaml", "Path to configuration file")
	flag.Usage = mghelper.Usage
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("error reading configuration file: %s", err.Error())
	}

	// Connect to database
	db := connectDB(&cfg.Database)
	defer db.Close()

	log.Printf("Running migrations for Relayer database (%s)...\n", cfg.Database.Database)

	// Get migrations
	migrationFns := relayerdb.GetMigrations()

	// Run migrations
	coll := migrations.NewCollection(migrationFns...).DisableSQLAutodiscover(true)
	oldVersion, newVersion, err := coll.Run(db, flag.Args()...)
	if err != nil {
		mghelper.Exitf(err.Error())
	}

	if newVersion != oldVersion {
		log.Printf("migrated from version %d to %d\n", oldVersion, newVersion)
	} else {
		log.Printf("version is %d\n", oldVersion)
	}
}

// connectDB creates a connection to the specified database
func connectDB(cfg *config.DatabaseConfig) *pg.DB {
	db := pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		User:     cfg.User,
		Password: cfg.Password,
		Database: cfg.Database,
	})

	// Test connection
	if err := db.Ping(db.Context()); err != nil {
		log.Fatalf("failed to connect to database %s: %s", cfg.Database, err.Error())
	}

	log.Printf("Successfully connected to database: %s", cfg.Database)
	return db
}
