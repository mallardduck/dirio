package service

import (
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/service/policy"
	"github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/internal/service/user"
	"github.com/mallardduck/dirio/internal/storage"
)

// ServicesFactory provides access to all service instances
type ServicesFactory struct {
	storage  *storage.Storage
	metadata *metadata.Manager

	// The actual services
	userService   *user.Service
	policyService *policy.Service
	s3Service     *s3.Service
}

// NewServiceFactory creates a new service factory with dependency injection
func NewServiceFactory(storage *storage.Storage, metadata *metadata.Manager) *ServicesFactory {
	return &ServicesFactory{
		storage:       storage,
		metadata:      metadata,
		userService:   user.NewService(metadata),
		policyService: policy.NewService(metadata),
		s3Service:     s3.NewService(storage, metadata),
	}
}

// User returns the user service
func (f *ServicesFactory) User() *user.Service {
	return f.userService
}

// Policy returns the policy service
func (f *ServicesFactory) Policy() *policy.Service {
	return f.policyService
}

// S3 returns the S3 service (buckets and objects)
func (f *ServicesFactory) S3() *s3.Service {
	return f.s3Service
}
