package main

import (
	"flag"
	"log"

	"github.com/chainsafe/canton-middleware/pkg/app/api"
	"github.com/chainsafe/canton-middleware/pkg/config"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	server := api.NewServer(cfg)

	if err = server.Run(); err != nil {
		log.Fatalf("api server exited with error: %v", err)
	}
}
