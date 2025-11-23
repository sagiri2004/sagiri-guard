package middleware

import (
	"fmt"
	"net/http"
	"sagiri-guard/backend/global"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	route  string
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) SetRoute(route string) { w.route = route }

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200, route: ""}
		next.ServeHTTP(sw, r)
		duration := time.Since(start)
		ip := r.RemoteAddr
		route := sw.route
		if route == "" {
			route = r.URL.Path
		}
		global.Logger.Info().
			Str("ip", ip).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("route", route).
			Int("status", sw.status).
			Dur("duration", duration).
			Msg("request")
		fmt.Printf("%s | %3d | %10s | %-6s %-40s -> %s\n",
			time.Now().Format("2006/01/02 15:04:05"),
			sw.status,
			duration,
			r.Method,
			r.URL.Path,
			route)
	})
}
