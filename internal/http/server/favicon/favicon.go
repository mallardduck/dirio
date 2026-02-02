package favicon

import (
	_ "embed"
	"net/http"

	"github.com/mallardduck/dirio/internal/logging"
)

//go:embed favicon.ico
var faviconBytes []byte

// HandleFavicon serves the favicon.ico file
func HandleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(faviconBytes)
	if err != nil {
		logging.Component("favicon-handler").With("error", err).Error("Failed to write favicon")
	}
}
