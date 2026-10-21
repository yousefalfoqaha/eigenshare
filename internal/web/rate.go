package web

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter struct {
	mu         sync.Mutex
	clients    map[string]*client
	rate       rate.Limit
	burst      int
	ttl        time.Duration
	trustProxy bool
}

func NewRateLimiter(r rate.Limit, burst int, ttl time.Duration, trustProxy bool) *RateLimiter {
	return &RateLimiter{
		clients:    make(map[string]*client),
		rate:       r,
		burst:      burst,
		ttl:        ttl,
		trustProxy: trustProxy,
	}
}

func (l *RateLimiter) get(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	c, ok := l.clients[key]
	if !ok {
		c = &client{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.clients[key] = c
	}
	c.lastSeen = time.Now()
	return c.limiter
}

func (l *RateLimiter) sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for key, c := range l.clients {
		if time.Since(c.lastSeen) > l.ttl {
			delete(l.clients, key)
		}
	}
}

func (l *RateLimiter) StartCleanup(done <-chan struct{}) {
	ticker := time.NewTicker(l.ttl / 2)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.sweep()
			case <-done:
				return
			}
		}
	}()
}

func (l *RateLimiter) clientIP(r *http.Request) string {
	if l.trustProxy {
		if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
			return strings.TrimSpace(xrip)
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip, _, _ := strings.Cut(xff, ",")
			return strings.TrimSpace(ip)
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}

func (l *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		res := l.get(l.clientIP(r)).Reserve()
		if !res.OK() {
			http.Error(w, "rate limiter misconfigured", http.StatusInternalServerError)
			return
		}
		if delay := res.Delay(); delay > 0 {
			res.Cancel()
			secs := int(delay.Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
