package gdelt

import (
	"html"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hugocbb/sotero/internal/source"
)

const (
	colID         = 0
	colDate       = 1
	colCollection = 2
	colURL        = 4
	colThemes     = 7
	colLocations  = 9
	colOrgs       = 13
	colTone       = 15
	colExtras     = 26
	numCols       = 27

	collectionWeb = "1"
)

// parseRecord converte uma linha do GKG em Item, se ela for de uma página web
// e citar alguma organização monitorada.
func parseRecord(line string, orgs *matcher) (source.Item, bool) {
	cols := strings.Split(line, "\t")
	if len(cols) < numCols || cols[colCollection] != collectionWeb {
		return source.Item{}, false
	}
	matched := orgs.Match(splitList(cols[colOrgs]))
	if len(matched) == 0 {
		return source.Item{}, false
	}

	published, _ := time.Parse(stampLayout, cols[colDate])
	return source.Item{
		URL:         cols[colURL],
		Title:       pageTitle(cols[colExtras]),
		PublishedAt: published,
		Lang:        "en",
		Meta: map[string]any{
			"gkg_id":    cols[colID],
			"orgs":      matched,
			"themes":    splitList(cols[colThemes]),
			"countries": countries(cols[colLocations]),
			"tone":      tone(cols[colTone]),
		},
	}, true
}

func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ';' })
}

func pageTitle(extras string) string {
	_, rest, ok := strings.Cut(extras, "<PAGE_TITLE>")
	if !ok {
		return ""
	}
	title, _, _ := strings.Cut(rest, "</PAGE_TITLE>")
	return html.UnescapeString(strings.TrimSpace(title))
}

func countries(locations string) []string {
	seen := map[string]bool{}
	var codes []string
	for _, loc := range splitList(locations) {
		parts := strings.Split(loc, "#")
		if len(parts) < 3 || parts[2] == "" || seen[parts[2]] {
			continue
		}
		seen[parts[2]] = true
		codes = append(codes, parts[2])
	}
	return codes
}

func tone(field string) float64 {
	first, _, _ := strings.Cut(field, ",")
	v, _ := strconv.ParseFloat(first, 64)
	return v
}

// matcher reconhece organizações monitoradas pelo nome exato ou por um nome
// que comece com o termo seguido de espaço ("nvidia" casa "nvidia corp").
type matcher struct {
	exact    map[string]string
	prefixes []string
}

func newMatcher(terms []string) *matcher {
	m := &matcher{exact: make(map[string]string, len(terms))}
	for _, t := range terms {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		m.exact[t] = t
		m.prefixes = append(m.prefixes, t+" ")
	}
	return m
}

func (m *matcher) Match(orgs []string) []string {
	var found []string
	for _, org := range orgs {
		if term, ok := m.term(org); ok && !slices.Contains(found, term) {
			found = append(found, term)
		}
	}
	return found
}

func (m *matcher) term(org string) (string, bool) {
	if term, ok := m.exact[org]; ok {
		return term, true
	}
	for _, p := range m.prefixes {
		if strings.HasPrefix(org, p) {
			return p[:len(p)-1], true
		}
	}
	return "", false
}
