package fetch

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// hostLimiter garante um intervalo mínimo entre requisições ao mesmo host,
// sem travar requisições a hosts diferentes.
type hostLimiter struct {
	every time.Duration
	mu    sync.Mutex
	hosts map[string]*rate.Limiter
}

func newHostLimiter(every time.Duration) *hostLimiter {
	return &hostLimiter{every: every, hosts: make(map[string]*rate.Limiter)}
}

func (h *hostLimiter) Wait(ctx context.Context, host string) error {
	return h.limiter(host).Wait(ctx)
}

func (h *hostLimiter) limiter(host string) *rate.Limiter {
	h.mu.Lock()
	defer h.mu.Unlock()
	l, ok := h.hosts[host]
	if !ok {
		l = rate.NewLimiter(rate.Every(h.every), 1)
		h.hosts[host] = l
	}
	return l
}
