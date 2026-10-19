// Command web serve o painel do grafo em tempo real.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hugocbb/sotero/internal/config"
	"github.com/hugocbb/sotero/internal/store"
	"github.com/hugocbb/sotero/internal/web"
	"golang.org/x/sync/errgroup"
)

const (
	notifyChannel = "graph"
	relistenDelay = 2 * time.Second
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("web stopped", "err", err)
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

	hub := web.NewHub()
	srv := &http.Server{
		Addr:              cfg.Web.Addr,
		Handler:           web.New(db, hub, log),
		ReadHeaderTimeout: 10 * time.Second,
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		for ctx.Err() == nil {
			if err := db.Listen(ctx, notifyChannel, hub.Publish); err != nil {
				log.Warn("listen", "err", err)
			}
			time.Sleep(relistenDelay)
		}
		return nil
	})
	g.Go(func() error {
		log.Info("web started", "addr", cfg.Web.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	})
	return g.Wait()
}
