// Package collector coordena o motor de scraping: descobre notícias nas
// fontes, grava no repositório e baixa o texto com um pool de workers.
package collector

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/hugocbb/sotero/internal/fetch"
	"github.com/hugocbb/sotero/internal/source"
	"github.com/hugocbb/sotero/internal/worker"
	"golang.org/x/sync/errgroup"
)

const statusPending = "pending"

type Repository interface {
	worker.Queue
	Cursor(ctx context.Context, src string) (source.Cursor, error)
	Commit(ctx context.Context, src string, items []source.Item, cur source.Cursor) (int, error)
	Complete(ctx context.Context, id int64, body, lang string) error
}

type Downloader interface {
	Article(ctx context.Context, url string) (fetch.Article, error)
}

type Options struct {
	Workers     int
	PollEvery   time.Duration
	Lease       time.Duration
	MaxAttempts int
}

type Collector struct {
	repo    Repository
	dl      Downloader
	sources []source.Source
	opts    Options
	log     *slog.Logger
	wake    chan struct{}
}

func New(repo Repository, dl Downloader, sources []source.Source, opts Options, log *slog.Logger) *Collector {
	return &Collector{
		repo:    repo,
		dl:      dl,
		sources: sources,
		opts:    opts,
		log:     log,
		wake:    make(chan struct{}, 1),
	}
}

func (c *Collector) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	for _, src := range c.sources {
		g.Go(func() error { return c.discover(ctx, src) })
	}

	downloads := &worker.Pool{
		Name:        "download",
		Queue:       c.repo,
		Status:      statusPending,
		Handle:      c.download,
		Permanent:   func(err error) bool { return errors.Is(err, fetch.ErrPermanent) },
		Workers:     c.opts.Workers,
		PollEvery:   c.opts.PollEvery,
		Lease:       c.opts.Lease,
		MaxAttempts: c.opts.MaxAttempts,
		Wake:        c.wake,
		Log:         c.log,
	}
	g.Go(func() error { return downloads.Run(ctx) })

	return g.Wait()
}

func (c *Collector) download(ctx context.Context, job worker.Job) error {
	art, err := c.dl.Article(ctx, job.URL)
	if err != nil {
		return err
	}
	return c.repo.Complete(ctx, job.ID, art.Text, art.Lang)
}

func (c *Collector) notify() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
