package middleware

import (
	"log"
	"net/http"
	"strings"
	"time"
)

func WithRateLimiting(bannedIps map[string]time.Time) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			addr := strings.Split(r.RemoteAddr, ":")
			ipAddr := addr[0]

			log.Println(ipAddr)
			next.ServeHTTP(w, r)
		})
	}
}
