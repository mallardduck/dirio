package service

import (
	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/persistence/storage"
	policyEngine "github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/service/group"
	"github.com/mallardduck/dirio/internal/service/policy"
	"github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/internal/service/serviceaccount"
	"github.com/mallardduck/dirio/internal/service/user"
)

// ServicesFactory provides access to all service instances
type ServicesFactory struct {
	diskStorage     *storage.Storage
	metadataManager *metadata.Manager
	policyEngine    *policyEngine.Engine
	authenticator   *auth.Authenticator

	// The actual services
	userService           *user.Service
	policyService         *policy.Service
	s3Service             *s3.Service
	groupService          *group.Service
	serviceAccountService *serviceaccount.Service
}

// NewServiceFactory creates a new service factory with dependency injection
func NewServiceFactory(
	diskStorage *storage.Storage,
	metadataManager *metadata.Manager,
	engine *policyEngine.Engine,
	authenticator *auth.Authenticator,
) *ServicesFactory {
	return &ServicesFactory{
		diskStorage:           diskStorage,
		metadataManager:       metadataManager,
		policyEngine:          engine,
		authenticator:         authenticator,
		userService:           user.NewService(metadataManager, authenticator),
		policyService:         policy.NewService(metadataManager),
		s3Service:             s3.NewService(diskStorage, metadataManager, engine),
		groupService:          group.NewService(metadataManager),
		serviceAccountService: serviceaccount.NewService(metadataManager),
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

// PolicyEngine returns the policy evaluation engine.
func (f *ServicesFactory) PolicyEngine() *policyEngine.Engine {
	return f.policyEngine
}

// Authenticator returns the authenticator service
func (f *ServicesFactory) Authenticator() *auth.Authenticator {
	return f.authenticator
}

// Group returns the group service
func (f *ServicesFactory) Group() *group.Service {
	return f.groupService
}

// ServiceAccount returns the service account service
func (f *ServicesFactory) ServiceAccount() *serviceaccount.Service {
	return f.serviceAccountService
}
