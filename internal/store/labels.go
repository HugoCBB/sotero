package store

import (
	"context"

	"github.com/hugocbb/sotero/internal/classify"
	"github.com/hugocbb/sotero/internal/graph"
	"github.com/jackc/pgx/v5"
)

// SaveResult registra a classificação (guardada também como exemplo de
// treino), refaz os eventos do artigo, propõe as relações encontradas e tira
// o artigo da fila. Tudo numa transação.
func (s *Store) SaveResult(ctx context.Context, articleID int64, res classify.Result, labeler string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	batch.Queue(`
		INSERT INTO classifications (article_id, labeler, relevant, factual, event_type, reason, result)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		articleID, labeler, res.Relevant, res.Factual, res.EventType, res.Reason, res)
	batch.Queue(`DELETE FROM events WHERE article_id = $1`, articleID)

	if res.Relevant {
		for _, im := range res.Impacts {
			batch.Queue(`
				INSERT INTO events (article_id, company_id, direction, severity, factual, event_type, domain, seen_at)
				SELECT id, $2, $3, $4, $5, $6, split_part(split_part(url, '://', 2), '/', 1), first_seen_at
				FROM articles WHERE id = $1`,
				articleID, im.Company, im.Direction, im.Severity, res.Factual, res.EventType)
		}
	}
	for _, rel := range res.Relations {
		batch.Queue(`
			INSERT INTO edges (src, dst, product, status) VALUES ($1, $2, $3, $4)
			ON CONFLICT (src, dst) DO NOTHING`,
			rel.Supplier, rel.Customer, rel.Product, graph.StatusProposed)
		batch.Queue(`
			INSERT INTO evidence (edge_id, article_id)
			SELECT id, $3 FROM edges WHERE src = $1 AND dst = $2
			ON CONFLICT DO NOTHING`,
			rel.Supplier, rel.Customer, articleID)
	}

	status := "rejected"
	if res.Relevant {
		status = "classified"
	}
	batch.Queue(`UPDATE articles SET status = $2, last_error = NULL WHERE id = $1`, articleID, status)

	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) LatestResult(ctx context.Context, articleID int64) (classify.Result, error) {
	var res classify.Result
	err := s.pool.QueryRow(ctx, `
		SELECT result FROM classifications WHERE article_id = $1 ORDER BY id DESC LIMIT 1`,
		articleID).Scan(&res)
	return res, err
}
