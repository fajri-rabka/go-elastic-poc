// Command demo walks through Phase 3: ensure the motorcycle index exists,
// store one sample document, and read it back by ID. It is safe to run
// repeatedly: it never deletes, recreates, or overwrites anything.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/fajri-rabka/go-elasticsearch-poc/internal/config"
	"github.com/fajri-rabka/go-elasticsearch-poc/internal/model"
	"github.com/fajri-rabka/go-elasticsearch-poc/internal/repository/elastic"
)

// The demo sends a handful of small requests; one shared deadline keeps a
// stuck cluster from hanging it without timing each request separately.
const demoTimeout = 30 * time.Second

const sampleID = "honda-beat-001"

// sampleMotorcycle is simulated data for this learning POC, not an official
// Honda specification or price.
func sampleMotorcycle() model.Motorcycle {
	return model.Motorcycle{
		Brand:        "Honda",
		ModelName:    "BeAT",
		Category:     "matic",
		EngineCC:     110,
		Transmission: "automatic",
		PriceIDR:     19_000_000,
		Available:    true,
		Description:  "Motor matic Honda untuk mobilitas harian",
	}
}

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

	ctx, cancel := context.WithTimeout(context.Background(), demoTimeout)
	defer cancel()

	info, err := client.Info().Do(ctx)
	if err != nil {
		return fmt.Errorf("check elasticsearch connection to %s: %w", cfg.ElasticsearchURL, err)
	}
	fmt.Println("Elasticsearch connected")
	fmt.Printf("Cluster : %s\n", info.ClusterName)
	fmt.Printf("Version : %s\n", info.Version.Int)

	repo := elastic.NewMotorcycleRepository(client, cfg.ElasticsearchIndex)

	created, err := repo.EnsureIndex(ctx)
	if err != nil {
		return fmt.Errorf("ensure index: %w", err)
	}
	fmt.Printf("\nIndex  : %s\n", cfg.ElasticsearchIndex)
	if created {
		fmt.Println("Status : created (1 shard, 0 replicas, explicit mapping)")
	} else {
		fmt.Println("Status : already exists, mapping verified")
	}

	want := sampleMotorcycle()
	switch err := repo.Create(ctx, sampleID, want); {
	case err == nil:
		fmt.Println("\nMotorcycle saved")
	case errors.Is(err, elastic.ErrAlreadyExists):
		fmt.Println("\nMotorcycle already exists, left unchanged")
	default:
		return fmt.Errorf("save sample motorcycle: %w", err)
	}
	fmt.Printf("ID     : %s\n", sampleID)
	fmt.Printf("Model  : %s %s\n", want.Brand, want.ModelName)

	got, err := repo.Get(ctx, sampleID)
	if err != nil {
		return fmt.Errorf("read sample motorcycle: %w", err)
	}
	if got != want {
		return fmt.Errorf("document %q exists but differs from the sample and was not overwritten:\n  stored: %+v\n  sample: %+v",
			sampleID, got, want)
	}

	fmt.Println("\nMotorcycle fetched successfully (GET by ID)")
	fmt.Printf("Brand        : %s\n", got.Brand)
	fmt.Printf("Model        : %s\n", got.ModelName)
	fmt.Printf("Category     : %s\n", got.Category)
	fmt.Printf("Engine (cc)  : %d\n", got.EngineCC)
	fmt.Printf("Transmission : %s\n", got.Transmission)
	fmt.Printf("Price (IDR)  : %d\n", got.PriceIDR)
	fmt.Printf("Available    : %t\n", got.Available)
	fmt.Println("Check        : stored document matches the sample")

	return nil
}
