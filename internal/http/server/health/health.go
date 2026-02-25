package health

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-git/go-billy/v5"
)

// Pinger is satisfied by *metadata.Manager.
type Pinger interface {
	Ping() error
}

// Handler implements the /health, /health/ready, and /health/live endpoints.
type Handler struct {
	metadata  Pinger
	rootFS    billy.Filesystem
	startTime time.Time
}

// New creates a Handler.  metadata and rootFS are used for readiness probes.
func New(metadata Pinger, rootFS billy.Filesystem) *Handler {
	return &Handler{
		metadata:  metadata,
		rootFS:    rootFS,
		startTime: time.Now(),
	}
}

// componentStatus is one component inside the health response.
type componentStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// healthResponse is the JSON body returned by HandleHealth.
type healthResponse struct {
	Status     string                     `json:"status"`
	Uptime     string                     `json:"uptime"`
	Components map[string]componentStatus `json:"components"`
}

// HandleHealth returns a JSON summary of all component statuses.
// HTTP 200 when everything is healthy; 503 when one or more components are degraded.
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	dbErr := h.metadata.Ping()
	storageErr := h.checkStorage()

	components := make(map[string]componentStatus, 2)
	healthy := true

	if dbErr != nil {
		components["metadata_db"] = componentStatus{Status: "error", Error: dbErr.Error()}
		healthy = false
	} else {
		components["metadata_db"] = componentStatus{Status: "ok"}
	}

	if storageErr != nil {
		components["storage"] = componentStatus{Status: "error", Error: storageErr.Error()}
		healthy = false
	} else {
		components["storage"] = componentStatus{Status: "ok"}
	}

	status := "ok"
	code := http.StatusOK
	if !healthy {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	resp := healthResponse{
		Status:     status,
		Uptime:     time.Since(h.startTime).Round(time.Second).String(),
		Components: components,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleReady is a readiness probe: 200 when BoltDB and storage are accessible,
// 503 otherwise.  Used by load balancers and orchestrators to gate traffic.
func (h *Handler) HandleReady(w http.ResponseWriter, r *http.Request) {
	if err := h.metadata.Ping(); err != nil {
		http.Error(w, "metadata db unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := h.checkStorage(); err != nil {
		http.Error(w, "storage unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// HandleLive is a liveness probe: always 200 if the process is reachable and
// not deadlocked.  Kept as /healthz alias for backwards compatibility with
// Docker health checks and the performance test wait loop.
func (h *Handler) HandleLive(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// checkStorage attempts a directory read on the root filesystem to confirm it
// is mounted and accessible.
func (h *Handler) checkStorage() error {
	_, err := h.rootFS.ReadDir(".")
	return err
}
