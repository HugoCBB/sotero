// Package ollama implementa classify.Classifier com um LLM local servido
// pelo Ollama, usando saída estruturada por JSON Schema.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hugocbb/sotero/internal/classify"
	"github.com/hugocbb/sotero/internal/graph"
)

const maxTextBytes = 3000

const systemPrompt = `You classify news for a supply-chain risk monitor. Reply only with JSON that matches the schema.

- relevant: true only if the article can affect revenue, supply, demand, costs, operations or regulation of at least one listed company.
- factual: true if it reports a concrete new event or announcement; false for opinion, analysis without news, advertising, listings or old/archive pages.
- event_type: the main kind of event.
- impacts: listed companies directly affected. direction is 1 (positive), -1 (negative) or 0 (unclear).
  severity: 0 none, 1 minor, 2 material (can move quarterly results), 3 major (factory shutdown, export ban, loss of a key customer). Most news is 0 or 1.
  A company from the "located in a mentioned country" list is affected only if the event hits its operations there (disaster, blockade, sanctions or export controls on its sector). General news about a country is not an impact.
  Empty if no listed company is affected.
- relations: supply relationships stated explicitly in the text between listed companies (supplier sells product to customer). Never guess. Empty if none.
- reason: one short sentence explaining the decision.

Use only the company ids from the list.`

type Client struct {
	http  *http.Client
	url   string
	model string
}

func New(url, model string, timeout time.Duration) *Client {
	return &Client{
		http:  &http.Client{Timeout: timeout},
		url:   strings.TrimRight(url, "/"),
		model: model,
	}
}

func (c *Client) Name() string { return "ollama:" + c.model }

func (c *Client) Classify(ctx context.Context, in classify.Input) (classify.Result, error) {
	payload, err := json.Marshal(chatRequest{
		Model:   c.model,
		Stream:  false,
		Format:  schema(in.Candidates()),
		Options: map[string]any{"temperature": 0},
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt(in)},
		},
	})
	if err != nil {
		return classify.Result{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return classify.Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return classify.Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return classify.Result{}, fmt.Errorf("ollama: status %d: %s", resp.StatusCode, msg)
	}

	var chat chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chat); err != nil {
		return classify.Result{}, err
	}
	var res classify.Result
	if err := json.Unmarshal([]byte(chat.Message.Content), &res); err != nil {
		return classify.Result{}, fmt.Errorf("ollama: invalid json: %w", err)
	}
	return res, nil
}

type chatRequest struct {
	Model    string         `json:"model"`
	Stream   bool           `json:"stream"`
	Format   any            `json:"format"`
	Options  map[string]any `json:"options"`
	Messages []message      `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message message `json:"message"`
}

func userPrompt(in classify.Input) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", in.Article.Title)
	if !in.Article.PublishedAt.IsZero() {
		fmt.Fprintf(&b, "Published: %s\n", in.Article.PublishedAt.Format(time.DateOnly))
	}
	writeCompanies(&b, "Companies named in the text", in.Mentioned)
	writeCompanies(&b, "Companies located in a mentioned country", in.Located)
	b.WriteString("\nText:\n")
	b.WriteString(truncate(in.Article.Body, maxTextBytes))
	return b.String()
}

func writeCompanies(b *strings.Builder, title string, companies []graph.Company) {
	if len(companies) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s (id: name, country, sector):\n", title)
	for _, c := range companies {
		fmt.Fprintf(b, "- %s: %s, %s, %s\n", c.ID, c.Name, c.Country, c.Sector)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

func schema(candidates []graph.Company) map[string]any {
	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.ID
	}
	company := map[string]any{"type": "string", "enum": ids}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"relevant":   map[string]any{"type": "boolean"},
			"factual":    map[string]any{"type": "boolean"},
			"event_type": map[string]any{"type": "string", "enum": classify.EventTypes},
			"impacts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"company":   company,
						"direction": map[string]any{"type": "integer", "enum": []int{-1, 0, 1}},
						"severity":  map[string]any{"type": "integer", "enum": []int{0, 1, 2, 3}},
					},
					"required": []string{"company", "direction", "severity"},
				},
			},
			"relations": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"supplier": company,
						"customer": company,
						"product":  map[string]any{"type": "string"},
					},
					"required": []string{"supplier", "customer", "product"},
				},
			},
			"reason": map[string]any{"type": "string"},
		},
		"required": []string{"relevant", "factual", "event_type", "impacts", "relations", "reason"},
	}
}
