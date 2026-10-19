// Package source define a interface comum das fontes de notícia (RSS, GDELT,
// EDGAR), o Item que elas produzem e o Cursor que marca até onde cada uma leu.
package source

import (
	"context"
	"time"
)

// Item é uma notícia descoberta por uma fonte, ainda sem o texto completo.
type Item struct {
	URL         string
	Title       string
	PublishedAt time.Time
	Lang        string
	Meta        map[string]any
}

// Cursor é o estado opaco de leitura de uma fonte. Só a própria fonte
// conhece o formato; o resto do sistema apenas guarda e devolve.
type Cursor string

// Source é uma origem de notícias consultada periodicamente.
//
// Fetch devolve os itens novos desde cur e o próximo cursor. Em caso de erro
// no meio do caminho, pode devolver o que já leu junto com o erro: o chamador
// grava os itens e o cursor parcial e tenta o resto na próxima rodada.
type Source interface {
	Name() string
	Interval() time.Duration
	Fetch(ctx context.Context, cur Cursor) ([]Item, Cursor, error)
}
