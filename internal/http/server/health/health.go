package health

import "net/http"

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	// TODO make this an actual health check of some sort - ideally we import from a health check service
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
