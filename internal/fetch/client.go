// Package fetch baixa artigos e extrai o texto, respeitando um intervalo
// mínimo entre requisições ao mesmo domínio.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"codeberg.org/readeck/go-readability/v2"
	"golang.org/x/net/html/charset"
)

// ErrPermanent marca falhas que não adianta tentar de novo (404, paywall,
// conteúdo que não é HTML).
var ErrPermanent = errors.New("permanent")

type Article struct {
	Text string
	Lang string
}

type Client struct {
	http     *http.Client
	hosts    *hostLimiter
	maxBytes int64
}

func New(httpClient *http.Client, perHost time.Duration, maxBytes int64) *Client {
	return &Client{
		http:     httpClient,
		hosts:    newHostLimiter(perHost),
		maxBytes: maxBytes,
	}
}

func (c *Client) Article(ctx context.Context, rawURL string) (Article, error) {
	page, err := url.Parse(rawURL)
	if err != nil {
		return Article{}, fmt.Errorf("%w: %v", ErrPermanent, err)
	}
	if err := c.hosts.Wait(ctx, page.Hostname()); err != nil {
		return Article{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Article{}, fmt.Errorf("%w: %v", ErrPermanent, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Article{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return Article{}, err
	}

	body, err := charset.NewReader(io.LimitReader(resp.Body, c.maxBytes), resp.Header.Get("Content-Type"))
	if err != nil {
		return Article{}, fmt.Errorf("%w: %v", ErrPermanent, err)
	}
	doc, err := readability.FromReader(body, resp.Request.URL)
	if err != nil {
		return Article{}, fmt.Errorf("%w: %v", ErrPermanent, err)
	}

	var text strings.Builder
	if err := doc.RenderText(&text); err != nil {
		return Article{}, fmt.Errorf("%w: %v", ErrPermanent, err)
	}
	if text.Len() == 0 {
		return Article{}, fmt.Errorf("%w: empty article", ErrPermanent)
	}
	return Article{Text: text.String(), Lang: doc.Language()}, nil
}

func checkResponse(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return fmt.Errorf("status %d", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: status %d", ErrPermanent, resp.StatusCode)
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
		return fmt.Errorf("%w: content-type %q", ErrPermanent, mediaType)
	}
	return nil
}
