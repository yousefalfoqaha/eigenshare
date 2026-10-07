package middleware

import (
	"context"
	"net/http"
	"strings"

	"golang.org/x/time/rate"
)

func WithRateLimiting(ctx context.Context, tokenRate rate.Limit, burstRate int, clientIPs map[string]*rate.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			addr := strings.Split(r.RemoteAddr, ":")
			ipAddr := addr[0]

			limiter := clientIPs[ipAddr]

			if limiter == nil {
				limiter = rate.NewLimiter(tokenRate, burstRate)
			}

			limiter.Wait(ctx)

			next.ServeHTTP(w, r)
		})
	}
}
