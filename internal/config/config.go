// Package config carrega a configuração do sistema a partir de um JSON.
package config

import (
	"encoding/json"
	"os"
	"time"
)

type Config struct {
	DatabaseURL  string   `json:"database_url"`
	UserAgent    string   `json:"user_agent"`
	Workers      int      `json:"workers"`
	PollEvery    Duration `json:"poll_every"`
	Lease        Duration `json:"lease"`
	MaxAttempts  int      `json:"max_attempts"`
	HTTPTimeout  Duration `json:"http_timeout"`
	PerHostEvery Duration `json:"per_host_every"`
	MaxPageBytes int64    `json:"max_page_bytes"`
	RSS          []Feed   `json:"rss"`
	GDELT        GDELT    `json:"gdelt"`
	Classify     Classify `json:"classify"`
	Web          Web      `json:"web"`
}

type Feed struct {
	Name  string   `json:"name"`
	URL   string   `json:"url"`
	Every Duration `json:"every"`
}

type GDELT struct {
	Enabled bool     `json:"enabled"`
	Every   Duration `json:"every"`
	Orgs    []string `json:"orgs"`
}

type Classify struct {
	Enabled   bool     `json:"enabled"`
	Workers   int      `json:"workers"`
	OllamaURL string   `json:"ollama_url"`
	Model     string   `json:"model"`
	Timeout   Duration `json:"timeout"`
}

type Web struct {
	Addr string `json:"addr"`
}

// Duration aceita textos como "30s" ou "5m" no JSON.
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	d.Duration = v
	return err
}

// Load lê path sobre os valores padrão. A variável DATABASE_URL, se
// definida, tem prioridade sobre o arquivo.
func Load(path string) (Config, error) {
	cfg := Config{
		Workers:      16,
		PollEvery:    Duration{5 * time.Second},
		Lease:        Duration{5 * time.Minute},
		MaxAttempts:  5,
		HTTPTimeout:  Duration{20 * time.Second},
		PerHostEvery: Duration{2 * time.Second},
		MaxPageBytes: 5 << 20,
		GDELT:        GDELT{Every: Duration{5 * time.Minute}},
		Classify: Classify{
			Workers:   1,
			OllamaURL: "http://localhost:11434",
			Model:     "qwen2.5:3b",
			Timeout:   Duration{3 * time.Minute},
		},
		Web: Web{Addr: ":8080"},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	for i := range cfg.RSS {
		if cfg.RSS[i].Every.Duration == 0 {
			cfg.RSS[i].Every.Duration = 5 * time.Minute
		}
	}
	if env := os.Getenv("DATABASE_URL"); env != "" {
		cfg.DatabaseURL = env
	}
	return cfg, nil
}
