package favicon

import (
	_ "embed"
	"net/http"
)

//go:embed favicon.ico
var faviconBytes []byte

// FaviconHandler serves the favicon.ico file
func FaviconHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.WriteHeader(http.StatusOK)
	w.Write(faviconBytes)
}
