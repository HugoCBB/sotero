// Command collector roda 24h: descobre notícias, baixa o texto e classifica
// cada artigo.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/hugocbb/sotero/internal/classify"
	"github.com/hugocbb/sotero/internal/collector"
	"github.com/hugocbb/sotero/internal/config"
	"github.com/hugocbb/sotero/internal/fetch"
	"github.com/hugocbb/sotero/internal/ollama"
	"github.com/hugocbb/sotero/internal/source"
	"github.com/hugocbb/sotero/internal/source/gdelt"
	"github.com/hugocbb/sotero/internal/source/rss"
	"github.com/hugocbb/sotero/internal/store"
	"golang.org/x/sync/errgroup"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("collector stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	path := flag.String("config", "config.json", "arquivo de configuração")
	flag.Parse()

	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}

	httpClient := fetch.NewHTTPClient(cfg.UserAgent, cfg.HTTPTimeout.Duration)
	downloader := fetch.New(httpClient, cfg.PerHostEvery.Duration, cfg.MaxPageBytes)

	g, ctx := errgroup.WithContext(ctx)

	c := collector.New(db, downloader, sources(cfg, httpClient), collector.Options{
		Workers:     cfg.Workers,
		PollEvery:   cfg.PollEvery.Duration,
		Lease:       cfg.Lease.Duration,
		MaxAttempts: cfg.MaxAttempts,
	}, log)
	g.Go(func() error { return c.Run(ctx) })

	if cfg.Classify.Enabled {
		clf := ollama.New(cfg.Classify.OllamaURL, cfg.Classify.Model, cfg.Classify.Timeout.Duration)
		stage := classify.New(db, clf, classify.Options{
			Workers:     cfg.Classify.Workers,
			PollEvery:   cfg.PollEvery.Duration,
			Lease:       cfg.Classify.Timeout.Duration + cfg.Lease.Duration,
			MaxAttempts: cfg.MaxAttempts,
		}, log)
		g.Go(func() error { return stage.Run(ctx) })
	}

	log.Info("collector started", "workers", cfg.Workers, "classify", cfg.Classify.Enabled)
	return g.Wait()
}

func sources(cfg config.Config, httpClient *http.Client) []source.Source {
	var list []source.Source
	for _, f := range cfg.RSS {
		list = append(list, rss.New(f.Name, f.URL, f.Every.Duration, httpClient))
	}
	if cfg.GDELT.Enabled {
		list = append(list, gdelt.New(cfg.GDELT.Orgs, cfg.GDELT.Every.Duration, httpClient))
	}
	return list
}
