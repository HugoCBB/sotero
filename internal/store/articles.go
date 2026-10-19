package store

import (
	"context"
	"errors"
	"time"

	"github.com/hugocbb/sotero/internal/classify"
	"github.com/hugocbb/sotero/internal/source"
	"github.com/hugocbb/sotero/internal/worker"
	"github.com/jackc/pgx/v5"
)

func (s *Store) Cursor(ctx context.Context, src string) (source.Cursor, error) {
	var cur string
	err := s.pool.QueryRow(ctx, `SELECT cursor FROM source_cursors WHERE source = $1`, src).Scan(&cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return source.Cursor(cur), err
}

// Commit grava os itens novos e o cursor da fonte numa única transação e
// devolve quantos itens eram inéditos. URLs repetidas são ignoradas.
func (s *Store) Commit(ctx context.Context, src string, items []source.Item, cur source.Cursor) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	for _, it := range items {
		batch.Queue(`
			INSERT INTO articles (url, source, title, published_at, lang, meta)
			VALUES ($1, $2, $3, $4, $5, COALESCE($6, '{}'::jsonb))
			ON CONFLICT (url_hash) DO NOTHING`,
			it.URL, src, it.Title, nullTime(it.PublishedAt), it.Lang, it.Meta)
	}
	batch.Queue(`
		INSERT INTO source_cursors (source, cursor) VALUES ($1, $2)
		ON CONFLICT (source) DO UPDATE SET cursor = EXCLUDED.cursor, updated_at = now()`,
		src, string(cur))

	results := tx.SendBatch(ctx, batch)
	inserted := 0
	for range items {
		tag, err := results.Exec()
		if err != nil {
			results.Close()
			return 0, err
		}
		inserted += int(tag.RowsAffected())
	}
	if err := results.Close(); err != nil {
		return 0, err
	}
	return inserted, tx.Commit(ctx)
}

// Claim reserva até n artigos com status para processamento. A reserva vale
// por lease: se o processo morrer antes de concluir, o artigo volta para a
// fila sozinho quando o prazo vence.
func (s *Store) Claim(ctx context.Context, status string, n int, lease time.Duration) ([]worker.Job, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE articles a
		SET attempts = a.attempts + 1,
		    next_attempt_at = now() + make_interval(secs => $3)
		FROM (
			SELECT id FROM articles
			WHERE status = $1 AND next_attempt_at <= now()
			ORDER BY next_attempt_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		) q
		WHERE a.id = q.id
		RETURNING a.id, a.url, a.attempts`,
		status, n, lease.Seconds())
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[worker.Job])
}

// Complete grava o texto baixado e entrega o artigo à fila de classificação.
func (s *Store) Complete(ctx context.Context, id int64, body, lang string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE articles
		SET status = 'fetched', body = $2, fetched_at = now(), last_error = NULL,
		    attempts = 0, next_attempt_at = now(),
		    lang = CASE WHEN lang = '' THEN $3 ELSE lang END
		WHERE id = $1`,
		id, body, lang)
	return err
}

func (s *Store) Retry(ctx context.Context, id int64, at time.Time, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE articles SET next_attempt_at = $2, last_error = $3 WHERE id = $1`,
		id, at, reason)
	return err
}

func (s *Store) Fail(ctx context.Context, id int64, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE articles SET status = 'failed', last_error = $2 WHERE id = $1`,
		id, reason)
	return err
}

func (s *Store) Article(ctx context.Context, id int64) (classify.Article, error) {
	var (
		a         classify.Article
		published *time.Time
	)
	err := s.pool.QueryRow(ctx, `
		SELECT id, url, title, COALESCE(body, ''), published_at, first_seen_at
		FROM articles WHERE id = $1`, id).
		Scan(&a.ID, &a.URL, &a.Title, &a.Body, &published, &a.FirstSeenAt)
	if published != nil {
		a.PublishedAt = *published
	}
	return a, err
}
