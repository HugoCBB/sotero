package classify

import (
	"regexp"
	"time"
)

const (
	maxAge  = 72 * time.Hour
	minBody = 400
)

var archiveTitle = regexp.MustCompile(`(?i)^(january|february|march|april|may|june|july|august|september|october|november|december) (19|20)\d{2}\b`)

// Check aplica as regras baratas de validade e devolve o motivo da rejeição,
// ou "" se o artigo segue para o classificador.
func Check(a Article) string {
	switch {
	case len(a.Body) < minBody:
		return "short"
	case archiveTitle.MatchString(a.Title):
		return "archive-page"
	case !a.PublishedAt.IsZero() && a.FirstSeenAt.Sub(a.PublishedAt) > maxAge:
		return "stale"
	}
	return ""
}
