package main

import (
	"flag"
	"log"

	appindexer "github.com/chainsafe/canton-middleware/pkg/app/indexer"
	"github.com/chainsafe/canton-middleware/pkg/config"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.LoadIndexerServer(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err = appindexer.NewServer(cfg).Run(); err != nil {
		log.Fatalf("indexer server exited with error: %v", err)
	}
}
