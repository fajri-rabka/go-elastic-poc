package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/fajri-rabka/go-elasticsearch-poc/internal/config"
	"github.com/fajri-rabka/go-elasticsearch-poc/internal/repository/elastic"
)

const connectionTimeout = 10 * time.Second

func main() {
	log.SetFlags(0)

	if err := run(); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	client, err := elastic.NewClient(cfg.ElasticsearchURL)
	if err != nil {
		return fmt.Errorf("create elasticsearch client: %w", err)
	}
	defer func() {
		if err := client.Close(context.Background()); err != nil {
			log.Printf("close elasticsearch client: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()

	info, err := client.Info().Do(ctx)
	if err != nil {
		return fmt.Errorf("check elasticsearch connection to %s: %w", cfg.ElasticsearchURL, err)
	}

	fmt.Println("Elasticsearch connected")
	fmt.Printf("Cluster : %s\n", info.ClusterName)
	// The generated typed API exposes the "number" JSON field as Version.Int.
	fmt.Printf("Version : %s\n", info.Version.Int)

	return nil
}
