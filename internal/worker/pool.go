// Package worker processa filas de artigos no Postgres com um pool de
// goroutines, reserva por prazo e retry com backoff.
package worker

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	retryBase = 30 * time.Second
	retryMax  = time.Hour
)

type Job struct {
	ID       int64
	URL      string
	Attempts int
}

type Queue interface {
	Claim(ctx context.Context, status string, n int, lease time.Duration) ([]Job, error)
	Retry(ctx context.Context, id int64, at time.Time, reason string) error
	Fail(ctx context.Context, id int64, reason string) error
}

// Pool consome os artigos com Status da Queue. Handle devolve erro para
// pedir nova tentativa; Permanent diz quais erros não valem nova tentativa.
type Pool struct {
	Name        string
	Queue       Queue
	Status      string
	Handle      func(ctx context.Context, job Job) error
	Permanent   func(err error) bool
	Workers     int
	PollEvery   time.Duration
	Lease       time.Duration
	MaxAttempts int
	Wake        <-chan struct{}
	Log         *slog.Logger
}

func (p *Pool) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	jobs := make(chan Job)

	g.Go(func() error {
		defer close(jobs)
		return p.dispatch(ctx, jobs)
	})
	for range p.Workers {
		g.Go(func() error {
			for job := range jobs {
				p.process(ctx, job)
			}
			return nil
		})
	}
	return g.Wait()
}

// dispatch reserva artigos e entrega aos workers. Com a fila vazia, espera o
// próximo ciclo ou um aviso em Wake.
func (p *Pool) dispatch(ctx context.Context, jobs chan<- Job) error {
	for {
		batch, err := p.Queue.Claim(ctx, p.Status, p.Workers*2, p.Lease)
		if err != nil && ctx.Err() == nil {
			p.Log.Error("claim", "pool", p.Name, "err", err)
		}
		for _, job := range batch {
			select {
			case jobs <- job:
			case <-ctx.Done():
				return nil
			}
		}
		if len(batch) > 0 {
			continue
		}

		select {
		case <-ctx.Done():
			return nil
		case <-p.Wake:
		case <-time.After(p.PollEvery):
		}
	}
}

func (p *Pool) process(ctx context.Context, job Job) {
	err := p.Handle(ctx, job)
	if err == nil || ctx.Err() != nil {
		return
	}

	log := p.Log.With("pool", p.Name, "id", job.ID, "url", job.URL, "attempt", job.Attempts, "err", err)
	if p.Permanent(err) || job.Attempts >= p.MaxAttempts {
		log.Warn("job failed")
		err = p.Queue.Fail(ctx, job.ID, err.Error())
	} else {
		log.Debug("job retry")
		err = p.Queue.Retry(ctx, job.ID, time.Now().Add(Backoff(job.Attempts)), err.Error())
	}
	if err != nil && ctx.Err() == nil {
		log.Error("save job result", "save_err", err)
	}
}

// Backoff dobra a espera a cada tentativa, até retryMax, com jitter para
// não sincronizar tentativas.
func Backoff(attempt int) time.Duration {
	d := retryBase << min(attempt-1, 10)
	d = min(d, retryMax)
	return d/2 + rand.N(d/2)
}
