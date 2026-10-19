// Package canon normaliza URLs para que a mesma notícia tenha sempre o mesmo
// endereço, base da deduplicação.
package canon

import (
	"errors"
	"net/url"
	"strings"
)

// ErrInvalid indica uma URL que não é http(s) absoluta.
var ErrInvalid = errors.New("canon: invalid url")

var trackingParams = map[string]bool{
	"fbclid": true, "gclid": true, "dclid": true, "msclkid": true,
	"mc_cid": true, "mc_eid": true, "igshid": true, "ocid": true,
	"cmpid": true, "smid": true, "ref": true, "ref_src": true,
	"at_campaign": true, "at_medium": true,
}

// URL devolve a forma canônica de raw: esquema e host em minúsculas, sem
// "www.", sem porta padrão, sem fragmento, sem parâmetros de rastreamento,
// query ordenada e sem barra ou "/amp" no fim do caminho.
func URL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", ErrInvalid
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", ErrInvalid
	}

	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if port := u.Port(); port != "" && !isDefaultPort(u.Scheme, port) {
		host += ":" + port
	}
	u.Host = host
	u.User = nil
	u.Fragment = ""
	u.RawFragment = ""

	u.Path = strings.TrimSuffix(u.Path, "/amp")
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""

	q := u.Query()
	for key := range q {
		if isTracking(key) {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func isTracking(key string) bool {
	key = strings.ToLower(key)
	return strings.HasPrefix(key, "utm_") || trackingParams[key]
}

func isDefaultPort(scheme, port string) bool {
	return (scheme == "http" && port == "80") || (scheme == "https" && port == "443")
}
