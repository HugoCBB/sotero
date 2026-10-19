package collector

import (
	"context"
	"time"

	"github.com/hugocbb/sotero/internal/canon"
	"github.com/hugocbb/sotero/internal/source"
)

func (c *Collector) discover(ctx context.Context, src source.Source) error {
	for {
		c.discoverOnce(ctx, src)
		if !sleep(ctx, src.Interval()) {
			return nil
		}
	}
}

// discoverOnce faz uma leitura da fonte e grava o que for novo. Erros são
// registrados e a fonte tenta de novo no próximo ciclo.
func (c *Collector) discoverOnce(ctx context.Context, src source.Source) {
	start := time.Now()
	log := c.log.With("source", src.Name())

	cur, err := c.repo.Cursor(ctx, src.Name())
	if err != nil {
		log.Error("read cursor", "err", err)
		return
	}

	items, next, fetchErr := src.Fetch(ctx, cur)
	if fetchErr != nil {
		log.Warn("fetch", "err", fetchErr)
	}
	if len(items) == 0 && next == cur {
		return
	}

	items = normalize(items)
	inserted, err := c.repo.Commit(ctx, src.Name(), items, next)
	if err != nil {
		log.Error("commit", "err", err)
		return
	}
	if inserted > 0 {
		c.notify()
	}
	log.Info("discovered", "seen", len(items), "new", inserted, "took_ms", time.Since(start).Milliseconds())
}

// normalize troca cada URL pela forma canônica e descarta as inválidas.
func normalize(items []source.Item) []source.Item {
	out := items[:0]
	for _, it := range items {
		u, err := canon.URL(it.URL)
		if err != nil {
			continue
		}
		it.URL = u
		out = append(out, it)
	}
	return out
}
