package handler

import (
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/refa-optical/backend/internal/app"
	"github.com/refa-optical/backend/internal/config"
)

var (
	initOnce   sync.Once
	apiHandler http.Handler
	initErr    error
)

// Handler is the Vercel Functions entrypoint. The application is initialized
// once per warm function instance, then reused for subsequent requests.
func Handler(w http.ResponseWriter, r *http.Request) {
	initOnce.Do(func() {
		cfg := config.Load()
		apiHandler, initErr = app.New(cfg)
		if initErr != nil {
			log.Printf("Vercel API initialization failed: %v", initErr)
		}
	})
	if initErr != nil {
		http.Error(w, "API initialization failed; check function logs and environment variables", http.StatusServiceUnavailable)
		return
	}

	// vercel.json rewrites customer-facing paths through this single function.
	// The original query string is preserved for catalog filters and searches.
	if route := r.URL.Query().Get("__vercel_path"); route != "" {
		query := r.URL.Query()
		query.Del("__vercel_path")
		r.URL.Path = "/" + strings.TrimLeft(route, "/")
		r.URL.RawPath = ""
		r.URL.RawQuery = query.Encode()
	}
	apiHandler.ServeHTTP(w, r)
}
