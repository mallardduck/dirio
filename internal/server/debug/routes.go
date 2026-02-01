package debug

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/mallardduck/dirio/internal/router"
)

// RouteEntry represents a single route in the debug output
type RouteEntry struct {
	Name    string `json:"name"`
	Method  string `json:"method"`
	Pattern string `json:"pattern"`
}

// RoutesResponse is the JSON response for the routes endpoint
type RoutesResponse struct {
	Routes []RouteEntry `json:"routes"`
}

// HandleRoutes returns a handler that lists all registered routes as JSON
func HandleRoutes(r *router.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		routes := r.RoutesWithMethods()

		// Convert to sorted slice
		entries := make([]RouteEntry, 0, len(routes))
		for name, info := range routes {
			entries = append(entries, RouteEntry{
				Name:    name,
				Method:  info.Method,
				Pattern: info.Pattern,
			})
		}

		// Sort by pattern first, then method
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Pattern != entries[j].Pattern {
				return entries[i].Pattern < entries[j].Pattern
			}
			return entries[i].Method < entries[j].Method
		})

		response := RoutesResponse{Routes: entries}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}
