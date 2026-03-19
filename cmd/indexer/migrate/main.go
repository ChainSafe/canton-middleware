package main

import (
	"flag"
	"log"

	"github.com/uptrace/bun/migrate"

	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/migrations/indexerdb"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Usage = mghelper.Usage
	flag.Parse()

	cfg, err := config.LoadIndexerServer(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := pgutil.ConnectDB(cfg.Database)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	log.Printf("Running migrations for indexer database (%s)...\n", cfg.Database.URL)

	if err = mghelper.RunMigrations(migrate.NewMigrator(db, indexerdb.Migrations), flag.Args()...); err != nil {
		mghelper.Exitf(err.Error())
	}
}
