package favicon

import (
	"bytes"
	_ "embed"
	"net/http"
	"time"

	"github.com/mallardduck/dirio/internal/version"
)

//go:embed favicon.ico
var faviconBytes []byte

// HandleFavicon serves the favicon.ico file
func HandleFavicon(w http.ResponseWriter, r *http.Request) {
	buildTime, err := time.Parse(time.RFC3339, version.BuildTime)
	if err != nil {
		buildTime = time.Now()
	}
	http.ServeContent(w, r, "favicon.ico", buildTime, bytes.NewReader(faviconBytes))
}
