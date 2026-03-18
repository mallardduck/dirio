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

	m.importPolicies(ctx, result.Policies)
	accessKeyToUUID := m.importUsers(ctx, result.Users)
	m.importBuckets(ctx, result.Buckets)
	m.importObjectMetadata(ctx, result.ObjectMetadata)

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

	m.importGroups(ctx, result.Groups, accessKeyToUUID)
	m.importServiceAccounts(ctx, result.ServiceAccounts, accessKeyToUUID)

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

func (m *Manager) importPolicies(ctx context.Context, policies map[string]*minio.Policy) {
	for _, p := range policies {
		var policyDoc PolicyDocument
		if err := jsonutil.Unmarshal([]byte(p.PolicyJSON), &policyDoc); err != nil {
			fmt.Printf("Warning: failed to parse policy document for %s: %v\n", p.Name, err)
			continue
		}
		pol := &Policy{
			Version:        iam.PolicyMetadataVersion,
			Name:           p.Name,
			PolicyDocument: &policyDoc,
			CreateDate:     p.CreateDate,
			UpdateDate:     p.UpdateDate,
		}
		if err := m.SavePolicy(ctx, pol); err != nil {
			fmt.Printf("Warning: failed to save policy %s: %v\n", p.Name, err)
		}
	}
	if len(policies) > 0 {
		fmt.Printf("Imported %d policies\n", len(policies))
	}
}

// importUsers saves each MinIO user and returns an accessKey→UUID map for downstream resolution.
func (m *Manager) importUsers(ctx context.Context, users map[string]*minio.User) map[string]uuid.UUID {
	accessKeyToUUID := make(map[string]uuid.UUID, len(users))
	for username, u := range users {
		userUUID := uuid.New()
		dirioUser := &User{
			Version:          iam.UserMetadataVersion,
			UUID:             userUUID,
			Username:         username,
			AccessKey:        u.AccessKey,
			SecretKey:        u.SecretKey,
			Status:           convertMinIOStatus(u.Status),
			UpdatedAt:        u.UpdatedAt,
			AttachedPolicies: u.AttachedPolicy,
		}
		if err := m.SaveUser(ctx, userUUID, dirioUser); err != nil {
			fmt.Printf("Warning: failed to save user %s: %v\n", username, err)
			continue
		}
		accessKeyToUUID[u.AccessKey] = userUUID
	}
	if len(users) > 0 {
		fmt.Printf("Imported %d users\n", len(users))
	}
	return accessKeyToUUID
}

func (m *Manager) importBuckets(ctx context.Context, buckets map[string]*minio.BucketMetadata) {
	for bucketName, b := range buckets {
		meta := &BucketMetadata{
			Version:                     BucketMetadataVersion,
			Name:                        b.Name,
			Owner:                       nil, // admin-owned; MinIO does not store bucket owner
			Created:                     b.Created,
			BucketPolicy:                parseBucketPolicy(bucketName, b.PolicyConfigJSON),
			NotificationConfigXML:       string(b.NotificationConfigXML),
			LifecycleConfigXML:          string(b.LifecycleConfigXML),
			ObjectLockConfigXML:         string(b.ObjectLockConfigXML),
			VersioningConfigXML:         string(b.VersioningConfigXML),
			EncryptionConfigXML:         string(b.EncryptionConfigXML),
			TaggingConfigXML:            string(b.TaggingConfigXML),
			QuotaConfigJSON:             string(b.QuotaConfigJSON),
			ReplicationConfigXML:        string(b.ReplicationConfigXML),
			BucketTargetsConfigJSON:     string(b.BucketTargetsConfigJSON),
			BucketTargetsConfigMetaJSON: string(b.BucketTargetsConfigMetaJSON),
			PolicyConfigUpdatedAt:       b.PolicyConfigUpdatedAt,
			ObjectLockConfigUpdatedAt:   b.ObjectLockConfigUpdatedAt,
			EncryptionConfigUpdatedAt:   b.EncryptionConfigUpdatedAt,
			TaggingConfigUpdatedAt:      b.TaggingConfigUpdatedAt,
			QuotaConfigUpdatedAt:        b.QuotaConfigUpdatedAt,
			ReplicationConfigUpdatedAt:  b.ReplicationConfigUpdatedAt,
			VersioningConfigUpdatedAt:   b.VersioningConfigUpdatedAt,
		}
		if err := m.saveBucketMetadata(ctx, bucketName, meta); err != nil {
			fmt.Printf("Warning: failed to save metadata for bucket %s: %v\n", bucketName, err)
		}
	}
	if len(buckets) > 0 {
		fmt.Printf("Imported %d buckets\n", len(buckets))
	}
}

func (m *Manager) importObjectMetadata(ctx context.Context, objects map[string]map[string]*minio.ObjectMetadata) {
	count := 0
	for bucketName, objs := range objects {
		for objectKey, minioMeta := range objs {
			dirioMeta := &ObjectMetadata{
				Version:        ObjectMetadataVersion,
				ContentType:    minioMeta.Meta["content-type"],
				ETag:           minioMeta.Meta["etag"],
				CustomMetadata: make(map[string]string),
			}
			for key, value := range minioMeta.Meta {
				if key != "content-type" && key != "etag" {
					dirioMeta.CustomMetadata[key] = value
				}
			}
			// Stat the actual object file to populate Size and LastModified.
			objPath := filepath.Join(bucketName, filepath.FromSlash(objectKey))
			if info, err := m.rootFS.Stat(objPath); err == nil {
				dirioMeta.Size = info.Size()
				dirioMeta.LastModified = info.ModTime()
			}
			if err := m.PutObjectMetadata(ctx, bucketName, objectKey, dirioMeta); err != nil {
				fmt.Printf("Warning: failed to save metadata for %s/%s: %v\n", bucketName, objectKey, err)
				continue
			}
			count++
		}
	}
	if count > 0 {
		fmt.Printf("Imported metadata for %d objects\n", count)
	}
}

func (m *Manager) importGroups(ctx context.Context, groups map[string]*minio.ImportGroup, accessKeyToUUID map[string]uuid.UUID) {
	count := 0
	for groupName, g := range groups {
		now := time.Now()
		updatedAt := g.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		memberUUIDs := make([]uuid.UUID, 0, len(g.Members))
		for _, accessKey := range g.Members {
			if uid, ok := accessKeyToUUID[accessKey]; ok {
				memberUUIDs = append(memberUUIDs, uid)
			} else {
				fmt.Printf("Warning: group %s member %q not found in imported users\n", groupName, accessKey)
			}
		}
		grp := &Group{
			Version:          iam.GroupMetadataVersion,
			Name:             groupName,
			Members:          memberUUIDs,
			AttachedPolicies: g.Policies,
			Status:           convertMinIOGroupStatus(g.Status),
			CreatedAt:        now,
			UpdatedAt:        updatedAt,
		}
		if err := m.SaveGroup(ctx, grp); err != nil {
			fmt.Printf("Warning: failed to save group %s: %v\n", groupName, err)
			continue
		}
		count++
	}
	if count > 0 {
		fmt.Printf("Imported %d groups\n", count)
	}
}

func (m *Manager) importServiceAccounts(ctx context.Context, sas map[string]*minio.ImportServiceAccount, accessKeyToUUID map[string]uuid.UUID) {
	count := 0
	for _, sa := range sas {
		now := time.Now()
		updatedAt := sa.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		uid, ok := accessKeyToUUID[sa.ParentUser]
		if !ok {
			fmt.Printf("Warning: service account %s parent %q not found in imported users; skipping\n",
				sa.AccessKey, sa.ParentUser)
			continue
		}
		parentUUID := &uid

		var expiresAt *time.Time
		if !sa.ExpiresAt.IsZero() {
			t := sa.ExpiresAt
			expiresAt = &t
		}

		policyMode, embeddedPolicyJSON := m.resolveSessionPolicy(sa)

		dirioSA := iam.NewServiceAccount(
			uuid.New(),
			sa.AccessKey,
			sa.SecretKey,
			sa.AccessKey,
			parentUUID,
			policyMode,
			convertMinIOSAStatus(sa.Status),
			embeddedPolicyJSON,
			expiresAt,
		)
		dirioSA.UpdatedAt = updatedAt
		if err := m.CreateServiceAccount(ctx, dirioSA); err != nil {
			fmt.Printf("Warning: failed to save service account %s: %v\n", sa.AccessKey, err)
			continue
		}
		count++
	}
	if count > 0 {
		fmt.Printf("Imported %d service accounts\n", count)
	}
}

// resolveSessionPolicy parses the MinIO session policy JSON and returns the appropriate
// policy mode and embedded JSON. Falls back to inherit on parse failure.
func (m *Manager) resolveSessionPolicy(sa *minio.ImportServiceAccount) (mode iam.PolicyMode, policyJSON string) {
	if sa.SessionPolicyJSON == "" {
		return iam.PolicyModeInherit, ""
	}
	var doc PolicyDocument
	if err := jsonutil.Unmarshal([]byte(sa.SessionPolicyJSON), &doc); err != nil {
		fmt.Printf("Warning: failed to parse session policy for %s: %v; falling back to inherit\n",
			sa.AccessKey, err)
		return iam.PolicyModeInherit, ""
	}
	return iam.PolicyModeOverride, sa.SessionPolicyJSON
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
