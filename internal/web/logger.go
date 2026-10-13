package web

import (
	"log"
	"net/http"
	"time"
)

type WrappedWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *WrappedWriter) WriteHeader(status int) {
	w.ResponseWriter.WriteHeader(status)
	w.status = status
	w.written = true
}

func (w *WrappedWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(b)
}

func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := &WrappedWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		start := time.Now()

		next.ServeHTTP(wrapped, r)

		log.Println(wrapped.status, r.Method, r.URL.Path, time.Since(start))
	})
}
