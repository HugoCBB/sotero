.PHONY: run db db-down ollama seed collector web

run:
	go run cmd/main.go

db:
	docker compose up -d postgres

db-down:
	docker compose down

ollama:
	docker compose up -d ollama
	docker compose exec ollama ollama pull qwen2.5:3b

seed: db
	go run ./cmd/seed -config config.json -file grafo.json

collector: db
	go run ./cmd/collector -config config.json

web: db
	go run ./cmd/web -config config.json
