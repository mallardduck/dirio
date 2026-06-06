package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/mallardduck/teapot-router/pkg/teapot"

	"github.com/mallardduck/dirio/console/ui"
	"github.com/mallardduck/dirio/consoleapi"
)

// BucketBrowser handles GET /buckets/{bucket}/objects — lists objects at the
// given prefix with "/" as delimiter (folder-style navigation).
func (h *Handler) BucketBrowser(w http.ResponseWriter, r *http.Request) {
	bucket := teapot.URLParam(r, "bucket")
	prefix := r.URL.Query().Get("prefix")
	objects, err := h.api.ListObjects(r.Context(), bucket, prefix, "/")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isHTMX(r) {
		render(w, r, ui.ObjectsTable(bucket, prefix, objects))
		return
	}
	render(w, r, ui.ObjectsPage(bucket, prefix, objects))
}

// ObjectDetail handles GET /buckets/{bucket}/objects/{key} — shows full
// object metadata, tags, and owner.
func (h *Handler) ObjectDetail(w http.ResponseWriter, r *http.Request) {
	bucket := teapot.URLParam(r, "bucket")
	key := teapot.URLParam(r, "key")
	meta, err := h.api.GetObjectMetadata(r.Context(), bucket, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	tags, _ := h.api.GetObjectTags(r.Context(), bucket, key)
	if tags == nil {
		tags = map[string]string{}
	}
	owner, _ := h.api.GetObjectOwner(r.Context(), bucket, key)
	render(w, r, ui.ObjectDetailPage(ui.ObjectDetailData{
		Bucket: bucket,
		Meta:   meta,
		Tags:   tags,
		Owner:  owner,
	}))
}

// ObjectDelete handles POST /buckets/{bucket}/objects/{key}/delete — deletes
// the object. HTMX requests get the refreshed table fragment; plain POST
// redirects back to the bucket browser at the parent prefix.
func (h *Handler) ObjectDelete(w http.ResponseWriter, r *http.Request) {
	bucket := teapot.URLParam(r, "bucket")
	key := teapot.URLParam(r, "key")
	prefix := r.URL.Query().Get("prefix")
	if prefix == "" {
		prefix = ui.ParentPrefix(key)
	}
	if err := h.api.DeleteObject(r.Context(), bucket, key); err != nil {
		h.triggerToast(w, "Failed to delete object: "+err.Error(), "error")
	} else {
		h.triggerToast(w, "Object deleted", "success")
	}
	if !isHTMX(r) {
		http.Redirect(w, r, string(ui.PageURL("/buckets/"+bucket+"/objects?prefix="+prefix)), http.StatusSeeOther)
		return
	}
	objects, _ := h.api.ListObjects(r.Context(), bucket, prefix, "/")
	render(w, r, ui.ObjectsTable(bucket, prefix, objects))
}

// ObjectCopy handles POST /buckets/{bucket}/objects/{key}/copy — copies the
// object to the destination bucket/key and redirects to the destination object.
func (h *Handler) ObjectCopy(w http.ResponseWriter, r *http.Request) {
	bucket := teapot.URLParam(r, "bucket")
	key := teapot.URLParam(r, "key")
	dstBucket := r.FormValue("dst_bucket")
	dstKey := r.FormValue("dst_key")
	if dstBucket == "" {
		dstBucket = bucket
	}
	if dstKey == "" {
		dstKey = key + ".copy"
	}
	if err := h.api.CopyObject(r.Context(), bucket, key, dstBucket, dstKey); err != nil {
		h.triggerToast(w, "Failed to copy object: "+err.Error(), "error")
		meta, _ := h.api.GetObjectMetadata(r.Context(), bucket, key)
		tags, _ := h.api.GetObjectTags(r.Context(), bucket, key)
		if tags == nil {
			tags = map[string]string{}
		}
		owner, _ := h.api.GetObjectOwner(r.Context(), bucket, key)
		render(w, r, ui.ObjectDetailPage(ui.ObjectDetailData{
			Bucket: bucket, Meta: meta, Tags: tags, Owner: owner,
		}))
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/buckets/"+dstBucket+"/objects/"+dstKey)), http.StatusSeeOther)
}

// ObjectSetTags handles POST /buckets/{bucket}/objects/{key}/tags — saves
// the key/value pairs submitted via the tags form and re-renders the detail page.
func (h *Handler) ObjectSetTags(w http.ResponseWriter, r *http.Request) {
	bucket := teapot.URLParam(r, "bucket")
	key := teapot.URLParam(r, "key")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	tagKeys := r.Form["tag_key"]
	tagValues := r.Form["tag_value"]
	tags := make(map[string]string, len(tagKeys))
	for i, k := range tagKeys {
		if k != "" && i < len(tagValues) {
			tags[k] = tagValues[i]
		}
	}
	if err := h.api.SetObjectTags(r.Context(), bucket, key, tags); err != nil {
		h.triggerToast(w, "Failed to save tags: "+err.Error(), "error")
	} else {
		h.triggerToast(w, "Tags saved", "success")
	}
	meta, _ := h.api.GetObjectMetadata(r.Context(), bucket, key)
	owner, _ := h.api.GetObjectOwner(r.Context(), bucket, key)
	render(w, r, ui.ObjectDetailPage(ui.ObjectDetailData{
		Bucket: bucket,
		Meta:   meta,
		Tags:   tags,
		Owner:  owner,
	}))
}

// ObjectPresignedURL handles POST /buckets/{bucket}/objects/{key}/presign —
// generates a presigned GET URL and returns it as an inline HTML fragment
// for HTMX to swap into the page.
func (h *Handler) ObjectPresignedURL(w http.ResponseWriter, r *http.Request) {
	bucket := teapot.URLParam(r, "bucket")
	key := teapot.URLParam(r, "key")
	accessKey, ok := h.sessions.Validate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	expiry := 15 * time.Minute
	if expiryStr := r.FormValue("expiry"); expiryStr != "" {
		if d, err := time.ParseDuration(expiryStr); err == nil && d > 0 {
			expiry = d
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := scheme + "://" + r.Host
	presignedURL, err := h.api.GeneratePresignedURL(r.Context(), consoleapi.GeneratePresignedURLRequest{
		AccessKey: accessKey,
		Bucket:    bucket,
		Key:       key,
		Expiry:    expiry,
		BaseURL:   baseURL,
		Method:    "GET",
	})
	if err != nil {
		h.triggerToast(w, "Failed to generate URL: "+err.Error(), "error")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	render(w, r, ui.PresignedURLResult(presignedURL))
}

// BucketUploadURL handles POST /buckets/{bucket}/upload-url — generates a
// presigned PUT URL for direct browser-to-S3 file upload and returns it as JSON.
func (h *Handler) BucketUploadURL(w http.ResponseWriter, r *http.Request) {
	bucket := teapot.URLParam(r, "bucket")
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
		return
	}
	accessKey, ok := h.sessions.Validate(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := scheme + "://" + r.Host
	presignedURL, err := h.api.GeneratePresignedURL(r.Context(), consoleapi.GeneratePresignedURLRequest{
		AccessKey: accessKey,
		Bucket:    bucket,
		Key:       key,
		Expiry:    15 * time.Minute,
		BaseURL:   baseURL,
		Method:    "PUT",
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"url": presignedURL})
}
