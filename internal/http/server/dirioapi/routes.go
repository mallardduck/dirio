package dirioapi

import (
	"github.com/mallardduck/teapot-router/pkg/teapot"
)

// RegisterRoutes mounts all DirIO REST API endpoints under /.dirio/api/v1/.
// Caller is responsible for wrapping r in auth middleware before calling this.
// When h is a stub (nil api), handlers return 200 OK (used by CLI route listing).
func RegisterRoutes(r *teapot.Router, h RouteHandlers) {
	// Ownership — bucket
	r.GET("/.dirio/api/v1/buckets/{bucket}/owner", h.HandleGetBucketOwner()).
		Name("dirioapi.buckets.owner.show").Action("dirio:GetBucketOwner")
	r.PUT("/.dirio/api/v1/buckets/{bucket}/owner", h.HandleTransferBucketOwner()).
		Name("dirioapi.buckets.owner.transfer").Action("dirio:TransferBucketOwner")

	// Ownership — object  ({key:.*} captures slashes)
	r.GET("/.dirio/api/v1/buckets/{bucket}/objects/{key:.*}", h.HandleGetObjectOwner()).
		Name("dirioapi.objects.owner.show").Action("dirio:GetObjectOwner")

	// Policy observability
	r.POST("/.dirio/api/v1/simulate", h.HandleSimulate()).
		Name("dirioapi.simulate").Action("dirio:Simulate")
	r.GET("/.dirio/api/v1/buckets/{bucket}/permissions/{accessKey}", h.HandleGetEffectivePermissions()).
		Name("dirioapi.buckets.permissions.show").Action("dirio:GetEffectivePermissions")
}
