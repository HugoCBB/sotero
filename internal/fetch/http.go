package fetch

import (
	"net/http"
	"time"
)

// NewHTTPClient cria o cliente HTTP compartilhado por fontes e downloads,
// com timeout e User-Agent fixos em toda requisição.
func NewHTTPClient(userAgent string, timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 4
	return &http.Client{
		Timeout:   timeout,
		Transport: userAgentTransport{agent: userAgent, next: transport},
	}
}

type userAgentTransport struct {
	agent string
	next  http.RoundTripper
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", t.agent)
	return t.next.RoundTrip(req)
}
