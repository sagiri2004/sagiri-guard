package middleware

import (
	"net/http"
	"sagiri-guard/backend/global"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		duration := time.Since(start)
		ip := r.RemoteAddr
		global.Logger.Info().Str("ip", ip).Str("method", r.Method).Str("path", r.URL.Path).Int("status", sw.status).Dur("duration", duration).Msg("request")
	})
}
