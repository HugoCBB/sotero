// Command seed carrega o grafo manual (grafo.json) no Postgres.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/hugocbb/sotero/internal/config"
	"github.com/hugocbb/sotero/internal/graph"
	"github.com/hugocbb/sotero/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", "config.json", "arquivo de configuração")
	graphPath := flag.String("file", "grafo.json", "arquivo do grafo")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*graphPath)
	if err != nil {
		return err
	}
	var f graph.File
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}

	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}
	if err := db.SeedGraph(ctx, f); err != nil {
		return err
	}
	fmt.Printf("%d empresas, %d arestas\n", len(f.Companies), len(f.Edges))
	return nil
}
