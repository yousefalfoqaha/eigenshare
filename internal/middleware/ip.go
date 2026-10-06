package middleware

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// reads the request ip
// takes a look at the in memory lookup
// show number of attempts on this endpoint
// a mathematical model that tells you remaining time
// so the lookup map is ip address as key value is the number of attempts + req url + time done
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
