package iam

import (
	"net/http"

	"github.com/mallardduck/dirio/internal/auth"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/router"
	"github.com/mallardduck/dirio/internal/storage"
)

// Handler handles IAM API requests
type Handler struct {
	storage  *storage.Storage
	metadata *metadata.Manager
	auth     *auth.Authenticator
}

func New(storage *storage.Storage, metadata *metadata.Manager, auth *auth.Authenticator) *Handler {
	return &Handler{
		storage:  storage,
		metadata: metadata,
		auth:     auth,
	}
}

func (h *Handler) UserResourceHandler() router.ResourceHandlers {
	return router.ResourceHandlers{
		Index: func(w http.ResponseWriter, r *http.Request) {
			// ListUsers
		},
		Store: func(w http.ResponseWriter, r *http.Request) {
			// CreateUser(s3)/StoreUser
		},
		Show: func(w http.ResponseWriter, r *http.Request) {
			// GetUser(s3)/ShowUser
		},
		Update: func(w http.ResponseWriter, r *http.Request) {
			// UpdateUser
		},
		Destroy: func(w http.ResponseWriter, r *http.Request) {
			// DeleteUser
		},
	}
}
