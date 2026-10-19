// Package gdelt implementa source.Source para o GKG do GDELT 2.0: a cada 15
// minutos baixa o arquivo novo e mantém só as notícias que citam as
// organizações monitoradas.
package gdelt

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/hugocbb/sotero/internal/source"
)

const (
	baseURL     = "https://data.gdeltproject.org/gdeltv2/"
	stampLayout = "20060102150405"
	step        = 15 * time.Minute
	maxCatchUp  = 96
	maxZipBytes = 256 << 20
	maxLine     = 16 << 20
)

var errNotFound = errors.New("gdelt: file not found")

type GKG struct {
	http     *http.Client
	orgs     *matcher
	interval time.Duration
}

func New(orgs []string, interval time.Duration, httpClient *http.Client) *GKG {
	return &GKG{http: httpClient, orgs: newMatcher(orgs), interval: interval}
}

func (g *GKG) Name() string { return "gdelt:gkg" }

func (g *GKG) Interval() time.Duration { return g.interval }

// Fetch processa todos os arquivos publicados depois de cur, até o mais
// recente, limitado às últimas 24h. O cursor é o carimbo do último arquivo.
func (g *GKG) Fetch(ctx context.Context, cur source.Cursor) ([]source.Item, source.Cursor, error) {
	latest, err := g.latest(ctx)
	if err != nil {
		return nil, cur, err
	}

	var items []source.Item
	for _, stamp := range pending(cur, latest) {
		batch, err := g.file(ctx, stamp)
		if err != nil && !errors.Is(err, errNotFound) {
			return items, cur, err
		}
		items = append(items, batch...)
		cur = source.Cursor(stamp.Format(stampLayout))
	}
	return items, cur, nil
}

func pending(cur source.Cursor, latest time.Time) []time.Time {
	from, err := time.Parse(stampLayout, string(cur))
	if err != nil {
		return []time.Time{latest}
	}
	from = from.Add(step)
	if oldest := latest.Add(-(maxCatchUp - 1) * step); from.Before(oldest) {
		from = oldest
	}
	var stamps []time.Time
	for t := from; !t.After(latest); t = t.Add(step) {
		stamps = append(stamps, t)
	}
	return stamps
}

func (g *GKG) latest(ctx context.Context) (time.Time, error) {
	body, err := g.get(ctx, baseURL+"lastupdate.txt", 4096)
	if err != nil {
		return time.Time{}, err
	}
	for line := range strings.Lines(string(body)) {
		fields := strings.Fields(line)
		if len(fields) == 3 && strings.HasSuffix(fields[2], ".gkg.csv.zip") {
			return time.Parse(stampLayout, strings.TrimSuffix(path.Base(fields[2]), ".gkg.csv.zip"))
		}
	}
	return time.Time{}, errors.New("gdelt: gkg not listed in lastupdate.txt")
}

func (g *GKG) file(ctx context.Context, stamp time.Time) ([]source.Item, error) {
	body, err := g.get(ctx, baseURL+stamp.Format(stampLayout)+".gkg.csv.zip", maxZipBytes)
	if err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, err
	}

	var items []source.Item
	for _, f := range archive.File {
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		batch, err := g.scan(r)
		r.Close()
		if err != nil {
			return nil, err
		}
		items = append(items, batch...)
	}
	return items, nil
}

func (g *GKG) scan(r io.Reader) ([]source.Item, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), maxLine)

	var items []source.Item
	for sc.Scan() {
		if it, ok := parseRecord(sc.Text(), g.orgs); ok {
			items = append(items, it)
		}
	}
	return items, sc.Err()
}

func (g *GKG) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return io.ReadAll(io.LimitReader(resp.Body, limit))
	case http.StatusNotFound:
		return nil, errNotFound
	default:
		return nil, fmt.Errorf("gdelt: %s: status %d", url, resp.StatusCode)
	}
}
