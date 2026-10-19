package web

import "sync"

// Hub repassa avisos de mudança no grafo para todos os navegadores
// conectados. Avisos para um cliente ocupado são descartados, pois o próximo
// recarregamento já traz o estado atual.
type Hub struct {
	mu      sync.Mutex
	clients map[chan struct{}]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan struct{}]struct{})}
}

func (h *Hub) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}
}

func (h *Hub) Publish() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
