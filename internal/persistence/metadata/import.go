package metadata

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/go-git/go-billy/v5/util"
	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/config/data"

	"github.com/mallardduck/dirio/internal/jsonutil"
	"github.com/mallardduck/dirio/internal/minio"
	"github.com/mallardduck/dirio/internal/persistence/path"
	"github.com/mallardduck/dirio/pkg/iam"
)

// ImportState tracks MinIO import status
type ImportState struct {
	Imported      bool      `json:"imported"`
	ImportedAt    time.Time `json:"importedAt"`
	MinIOModTime  time.Time `json:"minioModTime"`
	SourceVersion string    `json:"sourceVersion"`
}

// CheckAndImportMinIO checks for MinIO data and imports if needed.
// Returns true when this call performed the import (first time only), false
// when there was nothing to import or the data was already imported previously.
func (m *Manager) CheckAndImportMinIO(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context cancelled: %w", err)
	}
	// Check if .minio.sys exists
	if _, err := m.rootFS.Stat(path.MinIODir); err != nil {
		if isNotExist(err) {
			return false, nil // No MinIO data to import
		}
		return false, err
	}

	// Check import state
	state, err := m.getImportState()
	if err != nil {
		return false, err
	}

	if state.Imported {
		// Already imported, check if MinIO was modified since
		minioModTime, err := m.getMinIOModTime()
		if err == nil && minioModTime.After(state.MinIOModTime) {
			fmt.Printf("Warning: MinIO data modified after import. Consider re-importing.\n")
		}
		return false, nil
	}

	// Perform import using minio package
	fmt.Println("Detected MinIO data. Starting import...")

	// Get MinIO filesystem
	minioFS, err := path.NewMinIOFS(m.rootFS)
	if err != nil {
		return false, fmt.Errorf("failed to create MinIO filesystem: %w", err)
	}

	result, err := minio.Import(minioFS)
	if err != nil {
		return false, fmt.Errorf("MinIO import failed: %w", err)
	}

	// Convert and save policies
	if len(result.Policies) > 0 {
		for _, minioPolicy := range result.Policies {
			// Parse the policy JSON string into a PolicyDocument
			var policyDoc PolicyDocument
			if err := jsonutil.Unmarshal([]byte(minioPolicy.PolicyJSON), &policyDoc); err != nil {
				fmt.Printf("Warning: failed to parse policy document for %s: %v\n", minioPolicy.Name, err)
				continue
			}

			policy := &Policy{
				Version:        iam.PolicyMetadataVersion,
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

	// Convert and save users (one file per user).
	// Also build a local accessKey→UUID map for resolving group members and SA parents below.
	accessKeyToUUID := make(map[string]uuid.UUID, len(result.Users))
	if len(result.Users) > 0 {
		for username, minioUser := range result.Users {
			userUUID := uuid.New()
			dirioUser := &User{
				Version:          iam.UserMetadataVersion,
				UUID:             userUUID,
				Username:         username, // MinIO uses accessKey as username
				AccessKey:        minioUser.AccessKey,
				SecretKey:        minioUser.SecretKey,
				Status:           convertMinIOStatus(minioUser.Status),
				UpdatedAt:        minioUser.UpdatedAt,
				AttachedPolicies: minioUser.AttachedPolicy,
			}
			if err := m.SaveUser(ctx, userUUID, dirioUser); err != nil {
				fmt.Printf("Warning: failed to save user %s: %v\n", username, err)
				continue
			}
			accessKeyToUUID[minioUser.AccessKey] = userUUID
		}
		fmt.Printf("Imported %d users\n", len(result.Users))
	}

	// Convert and save buckets
	for bucketName, minioBucket := range result.Buckets {
		meta := &BucketMetadata{
			Version:      BucketMetadataVersion,
			Name:         minioBucket.Name,
			Owner:        nil, // nil = admin-only (MinIO doesn't store owner, assume admin created)
			Created:      minioBucket.Created,
			BucketPolicy: parseBucketPolicy(bucketName, minioBucket.PolicyConfigJSON),

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
	if len(result.Buckets) > 0 {
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

			// MinIO's fs.json has no size field — stat the actual object file to
			// populate Size and LastModified so HeadObject returns correct values.
			objPath := filepath.Join(bucketName, filepath.FromSlash(objectKey))
			if objInfo, statErr := m.rootFS.Stat(objPath); statErr == nil {
				dirioMeta.Size = objInfo.Size()
				dirioMeta.LastModified = objInfo.ModTime()
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

	// Save data config from MinIO import
	if result.DataConfig != nil {
		if err := data.SaveDataConfig(m.rootFS, result.DataConfig); err != nil {
			return false, fmt.Errorf("failed to save data config: %w", err)
		}
		fmt.Printf("Saved data config (region=%s, compression=%v)\n",
			result.DataConfig.Region,
			result.DataConfig.Compression.Enabled)
	}

	// Rebuild in-memory UUID index and bolt indexes so that users imported above
	// are immediately visible via GetUserByAccessKey / GetUserByUsername.
	// The indexes were built in New() before CheckAndImportMinIO was called, so
	// any users written by SaveUser() above are not yet indexed.
	m.buildUsersIndex(ctx)
	m.reconcileIndexes(ctx)

	// Convert and save groups.
	// Member access keys are resolved to UUIDs via the accessKeyToUUID map built above.
	groupCount := 0
	for groupName, minioGroup := range result.Groups {
		now := time.Now()
		updatedAt := minioGroup.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}

		// Resolve member access keys to UUIDs
		memberUUIDs := make([]uuid.UUID, 0, len(minioGroup.Members))
		for _, accessKey := range minioGroup.Members {
			if uid, ok := accessKeyToUUID[accessKey]; ok {
				memberUUIDs = append(memberUUIDs, uid)
			} else {
				fmt.Printf("Warning: group %s member %q not found in imported users\n", groupName, accessKey)
			}
		}

		g := &Group{
			Version:          iam.GroupMetadataVersion,
			Name:             groupName,
			Members:          memberUUIDs,
			AttachedPolicies: minioGroup.Policies,
			Status:           convertMinIOGroupStatus(minioGroup.Status),
			CreatedAt:        now,
			UpdatedAt:        updatedAt,
		}
		if err := m.SaveGroup(ctx, g); err != nil {
			fmt.Printf("Warning: failed to save group %s: %v\n", groupName, err)
			continue
		}
		groupCount++
	}
	if groupCount > 0 {
		fmt.Printf("Imported %d groups\n", groupCount)
	}

	// Convert and save service accounts.
	// Parent user access keys are resolved to UUIDs via accessKeyToUUID.
	saCount := 0
	for _, minioSA := range result.ServiceAccounts {
		now := time.Now()
		updatedAt := minioSA.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}

		var parentUUID *uuid.UUID
		if uid, ok := accessKeyToUUID[minioSA.ParentUser]; ok {
			parentUUID = &uid
		} else {
			fmt.Printf("Warning: service account %s parent %q not found in imported users; skipping\n",
				minioSA.AccessKey, minioSA.ParentUser)
			continue
		}

		var expiresAt *time.Time
		if !minioSA.ExpiresAt.IsZero() {
			t := minioSA.ExpiresAt
			expiresAt = &t
		}

		// If the service account had an embedded session policy, save it as a
		// Embed the session policy JSON directly on the SA (no ghost named policies).
		policyMode := iam.PolicyModeInherit
		embeddedPolicyJSON := ""
		if minioSA.SessionPolicyJSON != "" {
			// Validate it parses before committing to override mode.
			var sessionDoc PolicyDocument
			if err := jsonutil.Unmarshal([]byte(minioSA.SessionPolicyJSON), &sessionDoc); err != nil {
				fmt.Printf("Warning: failed to parse session policy for %s: %v; falling back to inherit\n",
					minioSA.AccessKey, err)
			} else {
				policyMode = iam.PolicyModeOverride
				embeddedPolicyJSON = minioSA.SessionPolicyJSON
			}
		}

		sa := iam.NewServiceAccount(
			uuid.New(),
			minioSA.AccessKey,
			minioSA.SecretKey,
			minioSA.AccessKey, // use accessKey as display name
			parentUUID,
			policyMode,
			convertMinIOSAStatus(minioSA.Status),
			embeddedPolicyJSON,
			expiresAt,
		)
		sa.UpdatedAt = updatedAt
		if err := m.CreateServiceAccount(ctx, sa); err != nil {
			fmt.Printf("Warning: failed to save service account %s: %v\n", minioSA.AccessKey, err)
			continue
		}
		saCount++
	}
	if saCount > 0 {
		fmt.Printf("Imported %d service accounts\n", saCount)
	}

	// Save import state
	state = &ImportState{
		Imported:      true,
		ImportedAt:    time.Now(),
		MinIOModTime:  time.Now(), // TODO: Get actual mod time
		SourceVersion: "RELEASE.2022-10-24T18-35-07Z",
	}
	if err := m.saveImportState(state); err != nil {
		return false, fmt.Errorf("failed to save import state: %w", err)
	}

	fmt.Println("MinIO import completed successfully")
	return true, nil
}

// getImportState retrieves the import state
func (m *Manager) getImportState() (*ImportState, error) {
	importData, err := util.ReadFile(m.metadataFS, ".import-state")
	if err != nil {
		if isNotExist(err) {
			return &ImportState{}, nil
		}
		return nil, err
	}

	var state ImportState
	if err := jsonutil.Unmarshal(importData, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// saveImportState saves the import state
func (m *Manager) saveImportState(state *ImportState) error {
	return jsonutil.MarshalToFile(m.metadataFS, ".import-state", state)
}

// convertMinIOGroupStatus maps MinIO group status strings to DirIO GroupStatus.
// Any unrecognised value defaults to disabled for safety.
func convertMinIOGroupStatus(minioStatus string) iam.GroupStatus {
	if minioStatus == "enabled" || minioStatus == "on" {
		return iam.GroupStatusActive
	}
	return iam.GroupStatusDisabled
}

// convertMinIOSAStatus maps MinIO service account status strings to DirIO ServiceAcctStatus.
// MinIO uses "on"/"off" for service accounts.
func convertMinIOSAStatus(minioStatus string) iam.ServiceAcctStatus {
	if minioStatus == "on" || minioStatus == "enabled" {
		return iam.ServiceAcctStatusActive
	}
	return iam.ServiceAcctStatusDisabled
}

// convertMinIOStatus maps MinIO user status strings ("enabled"/"disabled") to DirIO UserStatus ("on"/"off").
// Any unrecognised value defaults to disabled for safety.
func convertMinIOStatus(minioStatus string) iam.UserStatus {
	if minioStatus == "enabled" || minioStatus == "on" {
		return iam.UserStatusActive
	}
	return iam.UserStatusDisabled
}

// getMinIOModTime gets the last modification time of MinIO data
func (m *Manager) getMinIOModTime() (time.Time, error) {
	info, err := m.rootFS.Stat(path.MinIODir)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// parseBucketPolicy unmarshals a raw JSON bucket policy.
// Returns nil and logs a warning if policyJSON is empty or invalid.
func parseBucketPolicy(bucketName string, policyJSON []byte) *PolicyDocument {
	if len(policyJSON) == 0 {
		return nil
	}
	var policyDoc PolicyDocument
	if err := jsonutil.Unmarshal(policyJSON, &policyDoc); err != nil {
		fmt.Printf("Warning: failed to parse bucket policy for %s: %v\n", bucketName, err)
		return nil
	}
	return &policyDoc
}
