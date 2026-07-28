package api

import "net/http"

// MetricsAuthMiddleware protects /metrics with a single static bearer
// token, separate from the api key system used for /sandboxes - prometheus
// scrappers are configured with one static cred, not a per-user key
func MetricsAuthMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				http.Error(w, "metrics endpoint is disabled (METRICS_TOKEN not set)", http.StatusNotFound)
				return
			}

			provided := r.Header.Get("Authorization")
			if provided != "Bearer "+token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
