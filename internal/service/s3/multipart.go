package s3

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Multipart Upload Operations
// ============================================================================

// multipartMetadata stores metadata for a multipart upload
type multipartMetadata struct {
	UploadID       string            `json:"uploadId"`
	Bucket         string            `json:"bucket"`
	Key            string            `json:"key"`
	ContentType    string            `json:"contentType"`
	CustomMetadata map[string]string `json:"customMetadata"`
	Initiated      time.Time         `json:"initiated"`
}

// partMetadata stores metadata for an uploaded part
type partMetadata struct {
	PartNumber   int       `json:"partNumber"`
	ETag         string    `json:"etag"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
}

// CreateMultipartUpload initiates a multipart upload
func (s *Service) CreateMultipartUpload(ctx context.Context, req *CreateMultipartUploadRequest) (*CreateMultipartUploadResponse, error) {
	// Verify bucket exists
	bucketMeta, err := s.metadata.GetBucketMetadata(ctx, req.Bucket)
	if err != nil || bucketMeta == nil {
		return nil, errors.New("bucket does not exist")
	}

	// Generate unique upload ID
	uploadID := uuid.New().String()

	// Get bucket filesystem
	bucketFS, err := s.storage.GetBucketFS(ctx, req.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket filesystem: %w", err)
	}

	// Create multipart directory
	multipartDir := filepath.Join(".multipart", uploadID)
	if err := bucketFS.MkdirAll(multipartDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create multipart directory: %w", err)
	}

	// Store upload metadata
	uploadMeta := &multipartMetadata{
		UploadID:       uploadID,
		Bucket:         req.Bucket,
		Key:            req.Key,
		ContentType:    req.ContentType,
		CustomMetadata: req.CustomMetadata,
		Initiated:      time.Now(),
	}

	metaBytes, err := json.Marshal(uploadMeta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal upload metadata: %w", err)
	}

	metaPath := filepath.Join(multipartDir, "upload.json")
	metaFile, err := bucketFS.Create(metaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload metadata file: %w", err)
	}
	defer metaFile.Close()

	if _, err := metaFile.Write(metaBytes); err != nil {
		return nil, fmt.Errorf("failed to write upload metadata: %w", err)
	}

	return &CreateMultipartUploadResponse{
		UploadID: uploadID,
		Bucket:   req.Bucket,
		Key:      req.Key,
	}, nil
}

// UploadPart uploads a single part for a multipart upload
func (s *Service) UploadPart(ctx context.Context, req *UploadPartRequest) (*UploadPartResponse, error) {
	// Get bucket filesystem
	bucketFS, err := s.storage.GetBucketFS(ctx, req.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket filesystem: %w", err)
	}

	// Verify multipart upload exists
	multipartDir := filepath.Join(".multipart", req.UploadID)
	uploadMetaPath := filepath.Join(multipartDir, "upload.json")
	_, err = bucketFS.Stat(uploadMetaPath)
	if err != nil {
		return nil, errors.New("multipart upload not found")
	}

	// Create parts directory if needed
	partsDir := filepath.Join(multipartDir, "parts")
	if err := bucketFS.MkdirAll(partsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create parts directory: %w", err)
	}

	// Write part data
	partPath := filepath.Join(partsDir, fmt.Sprintf("part-%d.data", req.PartNumber))
	partFile, err := bucketFS.Create(partPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create part file: %w", err)
	}

	// Calculate ETag while writing
	hash := md5.New()
	writer := io.MultiWriter(partFile, hash)
	size, err := io.Copy(writer, req.Content)
	partFile.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to write part data: %w", err)
	}

	etag := hex.EncodeToString(hash.Sum(nil))

	// Store part metadata
	partMeta := &partMetadata{
		PartNumber:   req.PartNumber,
		ETag:         etag,
		Size:         size,
		LastModified: time.Now(),
	}

	metaBytes, err := json.Marshal(partMeta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal part metadata: %w", err)
	}

	partMetaPath := filepath.Join(partsDir, fmt.Sprintf("part-%d.json", req.PartNumber))
	partMetaFile, err := bucketFS.Create(partMetaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create part metadata file: %w", err)
	}
	defer partMetaFile.Close()

	if _, err := partMetaFile.Write(metaBytes); err != nil {
		return nil, fmt.Errorf("failed to write part metadata: %w", err)
	}

	return &UploadPartResponse{
		ETag: etag,
	}, nil
}

// CompleteMultipartUpload completes a multipart upload by assembling all parts
func (s *Service) CompleteMultipartUpload(ctx context.Context, req *CompleteMultipartUploadRequest) (*CompleteMultipartUploadResponse, error) {
	// Get bucket filesystem
	bucketFS, err := s.storage.GetBucketFS(ctx, req.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket filesystem: %w", err)
	}

	// Verify multipart upload exists and get metadata
	multipartDir := filepath.Join(".multipart", req.UploadID)
	uploadMetaPath := filepath.Join(multipartDir, "upload.json")
	uploadMetaFile, err := bucketFS.Open(uploadMetaPath)
	if err != nil {
		return nil, errors.New("multipart upload not found")
	}

	var uploadMeta multipartMetadata
	if err := json.NewDecoder(uploadMetaFile).Decode(&uploadMeta); err != nil {
		uploadMetaFile.Close()
		return nil, fmt.Errorf("failed to read upload metadata: %w", err)
	}
	uploadMetaFile.Close()

	// Sort parts by part number
	sort.Slice(req.Parts, func(i, j int) bool {
		return req.Parts[i].PartNumber < req.Parts[j].PartNumber
	})

	// Verify all parts exist and ETags match
	partsDir := filepath.Join(multipartDir, "parts")
	for _, part := range req.Parts {
		partMetaPath := filepath.Join(partsDir, fmt.Sprintf("part-%d.json", part.PartNumber))
		partMetaFile, err := bucketFS.Open(partMetaPath)
		if err != nil {
			return nil, fmt.Errorf("part %d not found", part.PartNumber)
		}

		var partMeta partMetadata
		if err := json.NewDecoder(partMetaFile).Decode(&partMeta); err != nil {
			partMetaFile.Close()
			return nil, fmt.Errorf("failed to read part %d metadata: %w", part.PartNumber, err)
		}
		partMetaFile.Close()

		// Normalize ETags for comparison (remove quotes if present)
		expectedETag := strings.Trim(partMeta.ETag, "\"")
		providedETag := strings.Trim(part.ETag, "\"")

		if expectedETag != providedETag {
			return nil, fmt.Errorf("part %d ETag mismatch: expected %s, got %s", part.PartNumber, expectedETag, providedETag)
		}
	}

	// Concatenate all parts into a buffer
	var assembledData bytes.Buffer
	for _, part := range req.Parts {
		partPath := filepath.Join(partsDir, fmt.Sprintf("part-%d.data", part.PartNumber))
		partFile, err := bucketFS.Open(partPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open part %d: %w", part.PartNumber, err)
		}

		if _, err := io.Copy(&assembledData, partFile); err != nil {
			partFile.Close()
			return nil, fmt.Errorf("failed to copy part %d: %w", part.PartNumber, err)
		}
		partFile.Close()
	}

	// Store the complete object
	etag, err := s.storage.PutObject(ctx, req.Bucket, req.Key, &assembledData, uploadMeta.ContentType, uploadMeta.CustomMetadata)
	if err != nil {
		return nil, fmt.Errorf("failed to store complete object: %w", err)
	}

	// Clean up multipart upload (best effort - don't fail request on cleanup error)
	_ = s.cleanupMultipartUpload(ctx, req.Bucket, req.UploadID)

	return &CompleteMultipartUploadResponse{
		Location: fmt.Sprintf("/%s/%s", req.Bucket, req.Key),
		Bucket:   req.Bucket,
		Key:      req.Key,
		ETag:     etag,
	}, nil
}

// AbortMultipartUpload aborts a multipart upload and cleans up all parts
func (s *Service) AbortMultipartUpload(ctx context.Context, req *AbortMultipartUploadRequest) error {
	// Verify multipart upload exists
	bucketFS, err := s.storage.GetBucketFS(ctx, req.Bucket)
	if err != nil {
		return fmt.Errorf("failed to get bucket filesystem: %w", err)
	}

	multipartDir := filepath.Join(".multipart", req.UploadID)
	uploadMetaPath := filepath.Join(multipartDir, "upload.json")
	_, err = bucketFS.Stat(uploadMetaPath)
	if err != nil {
		return errors.New("multipart upload not found")
	}

	// Clean up multipart upload
	return s.cleanupMultipartUpload(ctx, req.Bucket, req.UploadID)
}

// ListParts lists all parts for a multipart upload
func (s *Service) ListParts(ctx context.Context, req *ListPartsRequest) (*ListPartsResponse, error) {
	// Get bucket filesystem
	bucketFS, err := s.storage.GetBucketFS(ctx, req.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket filesystem: %w", err)
	}

	// Verify multipart upload exists
	multipartDir := filepath.Join(".multipart", req.UploadID)
	uploadMetaPath := filepath.Join(multipartDir, "upload.json")
	_, err = bucketFS.Stat(uploadMetaPath)
	if err != nil {
		return nil, errors.New("multipart upload not found")
	}

	// List all parts
	partsDir := filepath.Join(multipartDir, "parts")
	partFiles, err := bucketFS.ReadDir(partsDir)
	if err != nil {
		// No parts yet
		return &ListPartsResponse{
			Bucket:   req.Bucket,
			Key:      req.Key,
			UploadID: req.UploadID,
			Parts:    []Part{},
		}, nil
	}

	// Read part metadata
	responseParts := make([]Part, 0, len(partFiles))
	for _, fileInfo := range partFiles {
		if !strings.HasSuffix(fileInfo.Name(), ".json") {
			continue
		}

		partMetaPath := filepath.Join(partsDir, fileInfo.Name())
		partMetaFile, err := bucketFS.Open(partMetaPath)
		if err != nil {
			continue
		}

		var partMeta partMetadata
		if err := json.NewDecoder(partMetaFile).Decode(&partMeta); err != nil {
			partMetaFile.Close()
			continue
		}
		partMetaFile.Close()

		responseParts = append(responseParts, Part(partMeta))
	}

	// Sort by part number
	sort.Slice(responseParts, func(i, j int) bool {
		return responseParts[i].PartNumber < responseParts[j].PartNumber
	})

	return &ListPartsResponse{
		Bucket:   req.Bucket,
		Key:      req.Key,
		UploadID: req.UploadID,
		Parts:    responseParts,
	}, nil
}

// cleanupMultipartUpload removes all parts and metadata for a multipart upload
func (s *Service) cleanupMultipartUpload(ctx context.Context, bucket, uploadID string) error {
	// Get bucket filesystem
	bucketFS, err := s.storage.GetBucketFS(ctx, bucket)
	if err != nil {
		return fmt.Errorf("failed to get bucket filesystem: %w", err)
	}

	// Remove multipart directory
	multipartDir := filepath.Join(".multipart", uploadID)
	return bucketFS.Remove(multipartDir)
}
