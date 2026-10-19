// Package classify decide se um artigo baixado é um evento válido, quais
// empresas do grafo ele afeta e em que direção.
package classify

import (
	"context"
	"slices"
	"time"

	"github.com/hugocbb/sotero/internal/graph"
)

const (
	LabelerRules = "rules"
	LabelerHuman = "human"
)

var EventTypes = []string{
	"supply_disruption", "demand_change", "regulation", "geopolitical",
	"natural_disaster", "financial_results", "legal", "product", "other",
}

type Article struct {
	ID          int64
	URL         string
	Title       string
	Body        string
	PublishedAt time.Time
	FirstSeenAt time.Time
}

// Input separa as empresas citadas pelo nome das que só estão sediadas num
// país citado: estas só devem ser afetadas por eventos que atinjam as
// operações delas lá.
type Input struct {
	Article   Article
	Mentioned []graph.Company
	Located   []graph.Company
}

func (in Input) Candidates() []graph.Company {
	return append(slices.Clip(in.Mentioned), in.Located...)
}

type Impact struct {
	Company   string `json:"company"`
	Direction int    `json:"direction"`
	Severity  int    `json:"severity"`
}

type Relation struct {
	Supplier string `json:"supplier"`
	Customer string `json:"customer"`
	Product  string `json:"product"`
}

type Result struct {
	Relevant  bool       `json:"relevant"`
	Factual   bool       `json:"factual"`
	EventType string     `json:"event_type"`
	Impacts   []Impact   `json:"impacts"`
	Relations []Relation `json:"relations"`
	Reason    string     `json:"reason"`
}

type Classifier interface {
	Name() string
	Classify(ctx context.Context, in Input) (Result, error)
}

// RestrictTo descarta impactos de gravidade 0 e impactos e relações com
// empresas fora de candidates. Sem nenhum impacto restante, o resultado
// deixa de ser relevante.
func (r Result) RestrictTo(candidates []graph.Company) Result {
	known := func(id string) bool {
		return slices.ContainsFunc(candidates, func(c graph.Company) bool { return c.ID == id })
	}

	impacts := r.Impacts[:0:0]
	for _, im := range r.Impacts {
		if known(im.Company) && im.Severity > 0 {
			im.Direction = max(-1, min(1, im.Direction))
			im.Severity = min(3, im.Severity)
			impacts = append(impacts, im)
		}
	}
	relations := r.Relations[:0:0]
	for _, rel := range r.Relations {
		if known(rel.Supplier) && known(rel.Customer) && rel.Supplier != rel.Customer {
			relations = append(relations, rel)
		}
	}

	r.Impacts, r.Relations = impacts, relations
	r.Relevant = r.Relevant && len(impacts) > 0
	return r
}

// Corrected aplica a correção humana: valid=false invalida o artigo; com
// company, troca a direção do impacto nessa empresa. Relações não são
// repetidas, porque já foram propostas na classificação original.
func (r Result) Corrected(valid bool, company string, direction int) Result {
	r.Relevant = valid && len(r.Impacts) > 0
	r.Relations = nil
	r.Impacts = slices.Clone(r.Impacts)
	for i := range r.Impacts {
		if r.Impacts[i].Company == company {
			r.Impacts[i].Direction = max(-1, min(1, direction))
		}
	}
	return r
}
