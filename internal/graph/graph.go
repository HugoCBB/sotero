// Package graph define o grafo de dependências entre empresas e as visões
// dele servidas ao painel.
package graph

import "time"

const (
	StatusApproved = "approved"
	StatusProposed = "proposed"
	StatusRejected = "rejected"
)

// Company é uma empresa do grafo. Keywords são termos do negócio dela
// ("chip", "wafer"): uma notícia sobre o país só a afeta se citar algum.
type Company struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Country   string   `json:"country"`
	Sector    string   `json:"sector,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
	Keywords  []string `json:"keywords,omitempty"`
	Valuation float64  `json:"valuation,omitempty"`
}

// Edge diz que From fornece Product para To. Weight (0–1) é o quanto To
// depende de From.
type Edge struct {
	ID        int64   `json:"id,omitempty"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	Product   string  `json:"product"`
	Weight    float64 `json:"weight"`
	Exclusive bool    `json:"exclusive"`
	Status    string  `json:"status,omitempty"`
}

type File struct {
	Companies []Company `json:"companies"`
	Edges     []Edge    `json:"edges"`
}

// Node é uma empresa com o impacto agregado das notícias recentes. Impact
// vai de -1 a 1; Sources conta sites distintos, e menos de 2 significa
// notícia ainda não confirmada.
type Node struct {
	Company
	Impact  float64 `json:"impact"`
	Events  int     `json:"events"`
	Sources int     `json:"sources"`
}

type View struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Event struct {
	ArticleID int64     `json:"article_id"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Domain    string    `json:"domain"`
	SeenAt    time.Time `json:"seen_at"`
	EventType string    `json:"event_type"`
	Direction int       `json:"direction"`
	Severity  int       `json:"severity"`
	Factual   bool      `json:"factual"`
	Labeler   string    `json:"labeler"`
	Reason    string    `json:"reason"`
}
