package serve

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
)

func BearerAuth(token string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const prefix = "Bearer "
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, prefix) ||
				subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(authHeader, prefix)), []byte(token)) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				logger.WarnContext(r.Context(), "auth-middleware", "warn", "unauthorized access")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
