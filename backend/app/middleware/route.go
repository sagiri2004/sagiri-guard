package middleware

import "net/http"

type routeSetter interface {
	SetRoute(string)
}

// WithRoute tags the request/response with the route pattern before executing handler.
func WithRoute(pattern string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if setter, ok := w.(routeSetter); ok {
			setter.SetRoute(pattern)
		}
		next.ServeHTTP(w, r)
	})
}
