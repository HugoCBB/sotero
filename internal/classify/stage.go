package classify

import (
	"context"
	"log/slog"
	"time"

	"github.com/hugocbb/sotero/internal/graph"
	"github.com/hugocbb/sotero/internal/worker"
)

const statusFetched = "fetched"

type Repository interface {
	worker.Queue
	Article(ctx context.Context, id int64) (Article, error)
	Companies(ctx context.Context) ([]graph.Company, error)
	SaveResult(ctx context.Context, articleID int64, res Result, labeler string) error
}

type Options struct {
	Workers     int
	PollEvery   time.Duration
	Lease       time.Duration
	MaxAttempts int
}

// Stage consome os artigos baixados: regras primeiro, classificador só para
// o que passar e citar alguma empresa do grafo.
type Stage struct {
	repo Repository
	clf  Classifier
	opts Options
	log  *slog.Logger
}

func New(repo Repository, clf Classifier, opts Options, log *slog.Logger) *Stage {
	return &Stage{repo: repo, clf: clf, opts: opts, log: log}
}

func (s *Stage) Run(ctx context.Context) error {
	pool := &worker.Pool{
		Name:        "classify",
		Queue:       s.repo,
		Status:      statusFetched,
		Handle:      s.classify,
		Permanent:   func(error) bool { return false },
		Workers:     s.opts.Workers,
		PollEvery:   s.opts.PollEvery,
		Lease:       s.opts.Lease,
		MaxAttempts: s.opts.MaxAttempts,
		Log:         s.log,
	}
	return pool.Run(ctx)
}

func (s *Stage) classify(ctx context.Context, job worker.Job) error {
	art, err := s.repo.Article(ctx, job.ID)
	if err != nil {
		return err
	}
	if reason := Check(art); reason != "" {
		return s.repo.SaveResult(ctx, art.ID, Result{Reason: reason}, LabelerRules)
	}

	companies, err := s.repo.Companies(ctx)
	if err != nil {
		return err
	}
	in := Candidates(art, companies)
	if len(in.Mentioned)+len(in.Located) == 0 {
		return s.repo.SaveResult(ctx, art.ID, Result{Reason: "no-company"}, LabelerRules)
	}

	start := time.Now()
	res, err := s.clf.Classify(ctx, in)
	if err != nil {
		return err
	}
	res = res.RestrictTo(in.Candidates())

	s.log.Info("classified", "id", art.ID, "relevant", res.Relevant, "factual", res.Factual,
		"type", res.EventType, "impacts", len(res.Impacts), "relations", len(res.Relations),
		"took_ms", time.Since(start).Milliseconds())
	return s.repo.SaveResult(ctx, art.ID, res, s.clf.Name())
}
