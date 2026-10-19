package classify

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hugocbb/sotero/internal/graph"
)

const maxCandidates = 12

// Candidates monta a entrada do classificador com as empresas do grafo que
// o artigo pode afetar: as citadas pelo nome ou apelido e as sediadas num
// país citado, desde que o texto também cite um termo do negócio delas.
func Candidates(a Article, companies []graph.Company) Input {
	text := strings.ToLower(a.Title + "\n" + a.Body)

	in := Input{Article: a}
	for _, c := range companies {
		switch {
		case mentions(text, c):
			in.Mentioned = append(in.Mentioned, c)
		case containsWord(text, strings.ToLower(c.Country)) && containsAny(text, c.Keywords):
			in.Located = append(in.Located, c)
		}
	}

	in.Mentioned = in.Mentioned[:min(len(in.Mentioned), maxCandidates)]
	in.Located = in.Located[:min(len(in.Located), maxCandidates-len(in.Mentioned))]
	return in
}

func mentions(text string, c graph.Company) bool {
	return containsWord(text, strings.ToLower(c.Name)) || containsAny(text, c.Aliases)
}

func containsAny(text string, terms []string) bool {
	for _, t := range terms {
		if containsWord(text, strings.ToLower(t)) {
			return true
		}
	}
	return false
}

func containsWord(text, term string) bool {
	if term == "" {
		return false
	}
	for from := 0; ; {
		i := strings.Index(text[from:], term)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(term)
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		after, _ := utf8.DecodeRuneInString(text[end:])
		if !isWordRune(before) && !isWordRune(after) {
			return true
		}
		from = start + 1
	}
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
