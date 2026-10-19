package store

import (
	"context"
	"math"
	"time"

	"github.com/hugocbb/sotero/internal/graph"
	"github.com/jackc/pgx/v5"
)

func (s *Store) Companies(ctx context.Context) ([]graph.Company, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, country, sector, aliases, keywords, valuation FROM companies ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[graph.Company])
}

// SeedGraph grava empresas e arestas do arquivo como aprovadas,
// atualizando as que já existem.
func (s *Store) SeedGraph(ctx context.Context, f graph.File) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	for _, c := range f.Companies {
		batch.Queue(`
			INSERT INTO companies (id, name, country, sector, aliases, keywords, valuation)
			VALUES ($1, $2, $3, $4, COALESCE($5, '{}'::text[]), COALESCE($6, '{}'::text[]), $7)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, country = EXCLUDED.country, sector = EXCLUDED.sector,
				aliases = EXCLUDED.aliases, keywords = EXCLUDED.keywords, valuation = EXCLUDED.valuation`,
			c.ID, c.Name, c.Country, c.Sector, c.Aliases, c.Keywords, c.Valuation)
	}
	for _, e := range f.Edges {
		batch.Queue(`
			INSERT INTO edges (src, dst, product, weight, exclusive, status)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (src, dst) DO UPDATE SET product = EXCLUDED.product, weight = EXCLUDED.weight,
				exclusive = EXCLUDED.exclusive, status = EXCLUDED.status`,
			e.From, e.To, e.Product, e.Weight, e.Exclusive, graph.StatusApproved)
	}

	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// View monta o grafo com o impacto das notícias vistas dentro de window.
// Notícias não factuais pesam metade.
func (s *Store) View(ctx context.Context, window time.Duration) (graph.View, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.name, c.country, c.sector, c.aliases, c.keywords, c.valuation,
		       COALESCE(sum(e.direction * e.severity / 3.0 * CASE WHEN e.factual THEN 1 ELSE 0.5 END), 0),
		       count(e.article_id), count(DISTINCT e.domain)
		FROM companies c
		LEFT JOIN events e ON e.company_id = c.id AND e.seen_at > now() - make_interval(secs => $1)
		GROUP BY c.id
		ORDER BY c.id`, window.Seconds())
	if err != nil {
		return graph.View{}, err
	}
	nodes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (graph.Node, error) {
		var n graph.Node
		err := row.Scan(&n.ID, &n.Name, &n.Country, &n.Sector, &n.Aliases, &n.Keywords, &n.Valuation, &n.Impact, &n.Events, &n.Sources)
		n.Impact = math.Tanh(n.Impact)
		return n, err
	})
	if err != nil {
		return graph.View{}, err
	}

	rows, err = s.pool.Query(ctx, `
		SELECT id, src, dst, product, weight, exclusive, status
		FROM edges WHERE status <> $1 ORDER BY id`, graph.StatusRejected)
	if err != nil {
		return graph.View{}, err
	}
	edges, err := pgx.CollectRows(rows, pgx.RowToStructByPos[graph.Edge])
	if err != nil {
		return graph.View{}, err
	}
	return graph.View{Nodes: nodes, Edges: edges}, nil
}

func (s *Store) CompanyEvents(ctx context.Context, companyID string, window time.Duration) ([]graph.Event, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.title, a.url, e.domain, e.seen_at, e.event_type, e.direction, e.severity, e.factual,
		       cl.labeler, cl.reason
		FROM events e
		JOIN articles a ON a.id = e.article_id
		JOIN LATERAL (
			SELECT labeler, reason FROM classifications
			WHERE article_id = a.id ORDER BY id DESC LIMIT 1
		) cl ON true
		WHERE e.company_id = $1 AND e.seen_at > now() - make_interval(secs => $2)
		ORDER BY e.seen_at DESC
		LIMIT 50`, companyID, window.Seconds())
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[graph.Event])
}

func (s *Store) EdgeEvidence(ctx context.Context, edgeID int64) ([]graph.Event, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.title, a.url, split_part(split_part(a.url, '://', 2), '/', 1), a.first_seen_at,
		       '', 0, 0, cl.factual, cl.labeler, cl.reason
		FROM evidence ev
		JOIN articles a ON a.id = ev.article_id
		JOIN LATERAL (
			SELECT labeler, reason, factual FROM classifications
			WHERE article_id = a.id ORDER BY id DESC LIMIT 1
		) cl ON true
		WHERE ev.edge_id = $1
		ORDER BY a.first_seen_at DESC
		LIMIT 20`, edgeID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[graph.Event])
}

func (s *Store) SetEdgeStatus(ctx context.Context, edgeID int64, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE edges SET status = $2 WHERE id = $1`, edgeID, status)
	return err
}
