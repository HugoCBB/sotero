// Package web serve o painel do grafo: API JSON, atualizações em tempo real
// por Server-Sent Events e as correções humanas que alimentam o treino.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/hugocbb/sotero/internal/classify"
	"github.com/hugocbb/sotero/internal/graph"
)

const (
	defaultWindow = 24 * time.Hour
	maxWindow     = 30 * 24 * time.Hour
	keepAlive     = 25 * time.Second
)

//go:embed static
var static embed.FS

type Repository interface {
	View(ctx context.Context, window time.Duration) (graph.View, error)
	CompanyEvents(ctx context.Context, companyID string, window time.Duration) ([]graph.Event, error)
	EdgeEvidence(ctx context.Context, edgeID int64) ([]graph.Event, error)
	SetEdgeStatus(ctx context.Context, edgeID int64, status string) error
	LatestResult(ctx context.Context, articleID int64) (classify.Result, error)
	SaveResult(ctx context.Context, articleID int64, res classify.Result, labeler string) error
}

type Server struct {
	repo Repository
	hub  *Hub
	log  *slog.Logger
}

func New(repo Repository, hub *Hub, log *slog.Logger) http.Handler {
	s := &Server{repo: repo, hub: hub, log: log}
	assets, _ := fs.Sub(static, "static")

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(assets))
	mux.HandleFunc("GET /api/graph", s.graph)
	mux.HandleFunc("GET /api/stream", s.stream)
	mux.HandleFunc("GET /api/companies/{id}/events", s.companyEvents)
	mux.HandleFunc("GET /api/edges/{id}/evidence", s.edgeEvidence)
	mux.HandleFunc("POST /api/edges/{id}/status", s.edgeStatus)
	mux.HandleFunc("POST /api/articles/{id}/label", s.label)
	return mux
}

func (s *Server) graph(w http.ResponseWriter, r *http.Request) {
	view, err := s.repo.View(r.Context(), window(r))
	s.reply(w, view, err)
}

func (s *Server) companyEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.repo.CompanyEvents(r.Context(), r.PathValue("id"), window(r))
	s.reply(w, events, err)
}

func (s *Server) edgeEvidence(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	events, err := s.repo.EdgeEvidence(r.Context(), id)
	s.reply(w, events, err)
}

func (s *Server) edgeStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		(body.Status != graph.StatusApproved && body.Status != graph.StatusRejected) {
		http.Error(w, "status must be approved or rejected", http.StatusBadRequest)
		return
	}
	s.reply(w, body, s.repo.SetEdgeStatus(r.Context(), id, body.Status))
}

func (s *Server) label(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		Valid     bool   `json:"valid"`
		Company   string `json:"company"`
		Direction int    `json:"direction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	res, err := s.repo.LatestResult(r.Context(), id)
	if err != nil {
		s.reply(w, nil, err)
		return
	}
	res = res.Corrected(body.Valid, body.Company, body.Direction)
	s.reply(w, res, s.repo.SaveResult(r.Context(), id, res, classify.LabelerHuman))
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)

	updates, cancel := s.hub.Subscribe()
	defer cancel()

	ping := time.NewTicker(keepAlive)
	defer ping.Stop()

	send := func(msg string) bool {
		if _, err := w.Write([]byte(msg)); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send("event: ready\ndata: {}\n\n") {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-updates:
			if !send("event: graph\ndata: {}\n\n") {
				return
			}
		case <-ping.C:
			if !send(": ping\n\n") {
				return
			}
		}
	}
}

func (s *Server) reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.log.Error("api", "err", err)
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func window(r *http.Request) time.Duration {
	d, err := time.ParseDuration(r.URL.Query().Get("window"))
	if err != nil || d <= 0 {
		return defaultWindow
	}
	return min(d, maxWindow)
}
