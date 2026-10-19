// Package rss implementa source.Source para feeds RSS/Atom, usando GET
// condicional (ETag / Last-Modified) para não baixar o feed sem mudanças.
package rss

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hugocbb/sotero/internal/source"
	"github.com/mmcdole/gofeed"
)

type Feed struct {
	name     string
	url      string
	interval time.Duration
	http     *http.Client
	parser   *gofeed.Parser
}

type cursor struct {
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

// New cria um Feed chamado "rss:<name>" lido a cada interval.
func New(name, url string, interval time.Duration, httpClient *http.Client) *Feed {
	return &Feed{
		name:     "rss:" + name,
		url:      url,
		interval: interval,
		http:     httpClient,
		parser:   gofeed.NewParser(),
	}
}

func (f *Feed) Name() string { return f.name }

func (f *Feed) Interval() time.Duration { return f.interval }

// Fetch baixa o feed se ele mudou desde cur e devolve todos os itens dele.
// Itens já vistos são descartados pelo banco.
func (f *Feed) Fetch(ctx context.Context, cur source.Cursor) ([]source.Item, source.Cursor, error) {
	var prev cursor
	if cur != "" {
		_ = json.Unmarshal([]byte(cur), &prev)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, cur, err
	}
	if prev.ETag != "" {
		req.Header.Set("If-None-Match", prev.ETag)
	}
	if prev.LastModified != "" {
		req.Header.Set("If-Modified-Since", prev.LastModified)
	}

	resp, err := f.http.Do(req)
	if err != nil {
		return nil, cur, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, cur, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, cur, fmt.Errorf("%s: status %d", f.name, resp.StatusCode)
	}

	feed, err := f.parser.Parse(resp.Body)
	if err != nil {
		return nil, cur, fmt.Errorf("%s: %w", f.name, err)
	}

	next, _ := json.Marshal(cursor{
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	})
	return toItems(feed), source.Cursor(next), nil
}

func toItems(feed *gofeed.Feed) []source.Item {
	items := make([]source.Item, 0, len(feed.Items))
	for _, it := range feed.Items {
		items = append(items, source.Item{
			URL:         it.Link,
			Title:       it.Title,
			PublishedAt: publishedAt(it),
			Lang:        feed.Language,
		})
	}
	return items
}

func publishedAt(it *gofeed.Item) time.Time {
	switch {
	case it.PublishedParsed != nil:
		return it.PublishedParsed.UTC()
	case it.UpdatedParsed != nil:
		return it.UpdatedParsed.UTC()
	}
	return time.Time{}
}
