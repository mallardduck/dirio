package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-git/go-billy/v5/util"
	"github.com/mallardduck/dirio/internal/minio"
	"github.com/mallardduck/dirio/internal/path"
)

// ImportState tracks MinIO import status
type ImportState struct {
	Imported      bool      `json:"imported"`
	ImportedAt    time.Time `json:"importedAt"`
	MinIOModTime  time.Time `json:"minioModTime"`
	SourceVersion string    `json:"sourceVersion"`
}

// CheckAndImportMinIO checks for MinIO data and imports if needed
func (m *Manager) CheckAndImportMinIO(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}
	// Check if .minio.sys exists
	if _, err := m.rootFS.Stat(path.MinIODir); err != nil {
		if isNotExist(err) {
			return nil // No MinIO data to import
		}
		return err
	}

	// Check import state
	state, err := m.getImportState()
	if err != nil {
		return err
	}

	if state.Imported {
		// Already imported, check if MinIO was modified since
		minioModTime, err := m.getMinIOModTime()
		if err == nil && minioModTime.After(state.MinIOModTime) {
			fmt.Printf("Warning: MinIO data modified after import. Consider re-importing.\n")
		}
		return nil
	}

	// Perform import using minio package
	fmt.Println("Detected MinIO data. Starting import...")

	// Get MinIO filesystem
	minioFS, err := path.NewMinIOFS(m.rootFS)
	if err != nil {
		return fmt.Errorf("failed to create MinIO filesystem: %w", err)
	}

	result, err := minio.Import(minioFS)
	if err != nil {
		return fmt.Errorf("MinIO import failed: %w", err)
	}

	// Convert and save policies
	if len(result.Policies) > 0 {
		for _, minioPolicy := range result.Policies {
			// Parse the policy JSON string into a PolicyDocument
			var policyDoc PolicyDocument
			if err := json.Unmarshal([]byte(minioPolicy.PolicyJSON), &policyDoc); err != nil {
				fmt.Printf("Warning: failed to parse policy document for %s: %v\n", minioPolicy.Name, err)
				continue
			}

			policy := &Policy{
				Version:        PolicyMetadataVersion,
				Name:           minioPolicy.Name,
				PolicyDocument: &policyDoc,
				CreateDate:     minioPolicy.CreateDate,
				UpdateDate:     minioPolicy.UpdateDate,
			}
			if err := m.SavePolicy(ctx, policy); err != nil {
				fmt.Printf("Warning: failed to save policy %s: %v\n", minioPolicy.Name, err)
				continue
			}
		}
		fmt.Printf("Imported %d policies\n", len(result.Policies))
	}

	// Convert and save users
	if len(result.Users) > 0 {
		dirioUsers := make(map[string]*User)
		for username, minioUser := range result.Users {
			dirioUsers[username] = &User{
				Version:        UserMetadataVersion,
				AccessKey:      minioUser.AccessKey,
				SecretKey:      minioUser.SecretKey,
				Status:         minioUser.Status,
				UpdatedAt:      minioUser.UpdatedAt,
				AttachedPolicy: minioUser.AttachedPolicy,
			}
		}
		if err := m.SaveUsers(ctx, dirioUsers); err != nil {
			return fmt.Errorf("failed to save imported users: %w", err)
		}
		fmt.Printf("Imported %d users\n", len(dirioUsers))
	}

	// Convert and save buckets
	if len(result.Buckets) > 0 {
		for bucketName, minioBucket := range result.Buckets {
			// Parse bucket policy if present
			var bucketPolicy *PolicyDocument
			if len(minioBucket.PolicyConfigJSON) > 0 {
				var policyDoc PolicyDocument
				if err := json.Unmarshal(minioBucket.PolicyConfigJSON, &policyDoc); err != nil {
					fmt.Printf("Warning: failed to parse bucket policy for %s: %v\n", bucketName, err)
				} else {
					bucketPolicy = &policyDoc
				}
			}

			meta := &BucketMetadata{
				Version:      BucketMetadataVersion,
				Name:         minioBucket.Name,
				Owner:        "root", // MinIO doesn't store owner in bucket metadata
				Created:      minioBucket.Created,
				BucketPolicy: bucketPolicy,

				// Import all extended MinIO metadata fields
				NotificationConfigXML:       string(minioBucket.NotificationConfigXML),
				LifecycleConfigXML:          string(minioBucket.LifecycleConfigXML),
				ObjectLockConfigXML:         string(minioBucket.ObjectLockConfigXML),
				VersioningConfigXML:         string(minioBucket.VersioningConfigXML),
				EncryptionConfigXML:         string(minioBucket.EncryptionConfigXML),
				TaggingConfigXML:            string(minioBucket.TaggingConfigXML),
				QuotaConfigJSON:             string(minioBucket.QuotaConfigJSON),
				ReplicationConfigXML:        string(minioBucket.ReplicationConfigXML),
				BucketTargetsConfigJSON:     string(minioBucket.BucketTargetsConfigJSON),
				BucketTargetsConfigMetaJSON: string(minioBucket.BucketTargetsConfigMetaJSON),
				PolicyConfigUpdatedAt:       minioBucket.PolicyConfigUpdatedAt,
				ObjectLockConfigUpdatedAt:   minioBucket.ObjectLockConfigUpdatedAt,
				EncryptionConfigUpdatedAt:   minioBucket.EncryptionConfigUpdatedAt,
				TaggingConfigUpdatedAt:      minioBucket.TaggingConfigUpdatedAt,
				QuotaConfigUpdatedAt:        minioBucket.QuotaConfigUpdatedAt,
				ReplicationConfigUpdatedAt:  minioBucket.ReplicationConfigUpdatedAt,
				VersioningConfigUpdatedAt:   minioBucket.VersioningConfigUpdatedAt,
			}
			if err := m.saveBucketMetadata(ctx, bucketName, meta); err != nil {
				fmt.Printf("Warning: failed to save metadata for bucket %s: %v\n", bucketName, err)
				continue
			}
		}
		fmt.Printf("Imported %d buckets\n", len(result.Buckets))
	}

	// Convert and save object metadata
	objectCount := 0
	for bucketName, objects := range result.ObjectMetadata {
		for objectKey, minioMeta := range objects {
			// Convert MinIO metadata to DirIO format
			dirioMeta := &ObjectMetadata{
				Version:        ObjectMetadataVersion,
				ContentType:    minioMeta.Meta["content-type"],
				ETag:           minioMeta.Meta["etag"],
				CustomMetadata: make(map[string]string),
			}

			// Copy all metadata except content-type and etag (which have dedicated fields)
			for key, value := range minioMeta.Meta {
				if key != "content-type" && key != "etag" {
					dirioMeta.CustomMetadata[key] = value
				}
			}

			// Save the object metadata
			if err := m.PutObjectMetadata(ctx, bucketName, objectKey, dirioMeta); err != nil {
				fmt.Printf("Warning: failed to save metadata for %s/%s: %v\n", bucketName, objectKey, err)
				continue
			}
			objectCount++
		}
	}
	if objectCount > 0 {
		fmt.Printf("Imported metadata for %d objects\n", objectCount)
	}

	// Save import state
	state = &ImportState{
		Imported:      true,
		ImportedAt:    time.Now(),
		MinIOModTime:  time.Now(), // TODO: Get actual mod time
		SourceVersion: "RELEASE.2022-10-24T18-35-07Z",
	}
	if err := m.saveImportState(state); err != nil {
		return fmt.Errorf("failed to save import state: %w", err)
	}

	fmt.Println("MinIO import completed successfully")
	return nil
}

// getImportState retrieves the import state
func (m *Manager) getImportState() (*ImportState, error) {
	data, err := util.ReadFile(m.metadataFS, ".import-state")
	if err != nil {
		if isNotExist(err) {
			return &ImportState{}, nil
		}
		return nil, err
	}

	var state ImportState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// saveImportState saves the import state
func (m *Manager) saveImportState(state *ImportState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return util.WriteFile(m.metadataFS, ".import-state", data, 0644)
}

// getMinIOModTime gets the last modification time of MinIO data
func (m *Manager) getMinIOModTime() (time.Time, error) {
	info, err := m.rootFS.Stat(path.MinIODir)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
