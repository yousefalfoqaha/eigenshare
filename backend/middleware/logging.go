package middleware

import (
	"log"
	"net/http"
	"time"
)

type WrappedWriter struct {
	http.ResponseWriter
	status int
}

func (w *WrappedWriter) WriteHeader(status int) {
	w.ResponseWriter.WriteHeader(status)
	w.status = status
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrappedWriter := &WrappedWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		start := time.Now()

		next.ServeHTTP(wrappedWriter, r)

		log.Println(wrappedWriter.status, r.Method, r.URL.Path, time.Since(start))
	})
}
