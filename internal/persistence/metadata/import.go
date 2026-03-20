package metadata

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/go-git/go-billy/v5/util"
	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/consts"

	"github.com/mallardduck/dirio/internal/config/data"
	"github.com/mallardduck/dirio/internal/jsonutil"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/minio"
	"github.com/mallardduck/dirio/internal/persistence/path"
	"github.com/mallardduck/dirio/pkg/iam"
)

var importLog = logging.Component("import")

// ImportState tracks MinIO import status.
// Phase 1 (blocking): policies, users, buckets, object metadata → Imported=true.
// Phase 2 (async): groups, service accounts → AsyncComplete=true.
type ImportState struct {
	Imported         bool      `json:"imported"`
	ImportedAt       time.Time `json:"importedAt"`
	AsyncComplete    bool      `json:"asyncComplete"`
	AsyncCompletedAt time.Time `json:"asyncCompletedAt"`
	MinIOModTime     time.Time `json:"minioModTime"`
	SourceVersion    string    `json:"sourceVersion"`
}

// CheckAndImportMinIO checks for MinIO data and imports if needed.
//
// It returns three values:
//   - phase1Ran: true when Phase 1 (blocking) ran this call — callers use this
//     to know whether to reload DataConfig.
//   - asyncPhase: non-nil function containing Phase 2 work (groups, service
//     accounts) that should be run in a background goroutine after the server
//     starts. Nil when there is nothing left to import.
//   - err: any fatal error from Phase 1.
//
// Phase 1 (blocking): config, policies, users, buckets, object metadata.
// Phase 2 (async):    groups, service accounts.
//
// Restart recovery: if a previous run completed Phase 1 but was interrupted
// before Phase 2 finished, this call skips Phase 1 and returns only the async
// Phase 2 function (phase1Ran=false so DataConfig is not reloaded unnecessarily).
func (m *Manager) CheckAndImportMinIO(ctx context.Context) (phase1Ran bool, asyncPhase func(context.Context), err error) {

	if err := ctx.Err(); err != nil {
		return false, nil, fmt.Errorf("context cancelled: %w", err)
	}

	if _, err := m.rootFS.Stat(consts.MinioMetadataDir); err != nil {
		if isNotExist(err) {
			return false, nil, nil // No MinIO data present.
		}
		return false, nil, err
	}

	state, err := m.getImportState()
	if err != nil {
		return false, nil, err
	}

	// Fully done — nothing to do.
	if state.Imported && state.AsyncComplete {
		minioModTime, err := m.getMinIOModTime()
		if err == nil && minioModTime.After(state.MinIOModTime) {
			importLog.Warn("MinIO data modified after import; consider re-importing")
		}
		return false, nil, nil
	}

	// Phase 1 already done on a previous run but Phase 2 was interrupted.
	// Rebuild the accessKey→UUID map from BoltDB and return only the async func.
	if state.Imported && !state.AsyncComplete {
		importLog.Info("Phase 1 already complete; resuming async Phase 2 (groups, service accounts)")
		accessKeyToUUID := m.buildAccessKeyToUUIDMap(ctx)
		return false, m.makeAsyncPhase(state, accessKeyToUUID), nil
	}

	// --- Phase 1 (blocking) ---
	importLog.Info("detected MinIO data, starting Phase 1 import (policies, users, buckets, objects)")

	minioFS, err := path.NewMinIOFS(m.rootFS)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create MinIO filesystem: %w", err)
	}

	result, err := minio.Import(minioFS)
	if err != nil {
		return false, nil, fmt.Errorf("MinIO import failed: %w", err)
	}

	m.importPolicies(ctx, result.Policies)
	accessKeyToUUID := m.importUsers(ctx, result.Users)
	m.importBuckets(ctx, result.Buckets)

	if result.DataConfig != nil {
		if err := data.SaveDataConfig(m.rootFS, result.DataConfig); err != nil {
			return false, nil, fmt.Errorf("failed to save data config: %w", err)
		}
		importLog.Info("saved data config from MinIO import",
			"region", result.DataConfig.Region,
			"compression", result.DataConfig.Compression.Enabled,
		)
	}

	// Rebuild in-memory UUID index and bolt indexes so that users imported above
	// are immediately visible via GetUserByAccessKey / GetUserByUsername.
	m.buildUsersIndex(ctx)
	m.reconcileIndexes(ctx)

	now := time.Now()
	state = &ImportState{
		Imported:      true,
		ImportedAt:    now,
		AsyncComplete: false,
		MinIOModTime:  now,
		SourceVersion: "RELEASE.2022-10-24T18-35-07Z",
	}
	if err := m.saveImportState(state); err != nil {
		return false, nil, fmt.Errorf("failed to save import state after Phase 1: %w", err)
	}

	importLog.Info("Phase 1 import complete; Phase 2 (groups, service accounts) will run async")
	return true, m.makeAsyncPhase(state, accessKeyToUUID), nil
}

// makeAsyncPhase returns a closure that imports groups and service accounts
// and then marks AsyncComplete in the import state. Captures state and
// accessKeyToUUID by value so they survive the caller returning.
func (m *Manager) makeAsyncPhase(state *ImportState, accessKeyToUUID map[string]uuid.UUID) func(context.Context) {
	return func(ctx context.Context) {
		importLog.Info("starting async Phase 2 import (groups, service accounts)")

		minioFS, err := path.NewMinIOFS(m.rootFS)
		if err != nil {
			importLog.Error("failed to create MinIO filesystem for Phase 2", "error", err)
			return
		}

		result, err := minio.Import(minioFS)
		if err != nil {
			importLog.Error("MinIO re-read for Phase 2 failed", "error", err)
			return
		}

		m.importObjectMetadata(ctx, result.ObjectMetadata)
		m.importGroups(ctx, result.Groups, accessKeyToUUID)
		m.importServiceAccounts(ctx, result.ServiceAccounts, accessKeyToUUID)

		state.AsyncComplete = true
		state.AsyncCompletedAt = time.Now()
		if err := m.saveImportState(state); err != nil {
			importLog.Error("failed to save import state after Phase 2", "error", err)
			return
		}
		importLog.Info("async Phase 2 import complete")
	}
}

// buildAccessKeyToUUIDMap reconstructs the accessKey→UUID map by scanning all
// users already persisted in BoltDB. Used when Phase 1 completed in a prior
// run and Phase 2 needs to resume without re-importing users.
func (m *Manager) buildAccessKeyToUUIDMap(ctx context.Context) map[string]uuid.UUID {
	users, err := m.GetUsers(ctx)
	if err != nil {
		importLog.Error("failed to load users for Phase 2 resume", "error", err)
		return map[string]uuid.UUID{}
	}
	result := make(map[string]uuid.UUID, len(users))
	for uid, u := range users {
		result[u.AccessKey] = uid
	}
	return result
}

func (m *Manager) importPolicies(ctx context.Context, policies map[string]*minio.Policy) {
	for _, p := range policies {
		if _, err := m.GetPolicy(ctx, p.Name); err == nil {
			continue // already imported
		}
		var policyDoc PolicyDocument
		if err := jsonutil.Unmarshal([]byte(p.PolicyJSON), &policyDoc); err != nil {
			importLog.Warn("failed to parse policy document", "policy", p.Name, "error", err)
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
			importLog.Warn("failed to save policy", "policy", p.Name, "error", err)
		}
	}
	if len(policies) > 0 {
		importLog.Info("imported policies", "count", len(policies))
	}
}

// importUsers saves each MinIO user and returns an accessKey→UUID map for downstream resolution.
func (m *Manager) importUsers(ctx context.Context, users map[string]*minio.User) map[string]uuid.UUID {
	accessKeyToUUID := make(map[string]uuid.UUID, len(users))
	for username, u := range users {
		if existing, err := m.GetUserByAccessKey(ctx, u.AccessKey); err == nil {
			accessKeyToUUID[u.AccessKey] = existing.UUID // needed by Phase 2
			continue                                     // already imported
		}
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
			importLog.Warn("failed to save user", "username", username, "error", err)
			continue
		}
		accessKeyToUUID[u.AccessKey] = userUUID
	}
	if len(users) > 0 {
		importLog.Info("imported users", "count", len(users))
	}
	return accessKeyToUUID
}

func (m *Manager) importBuckets(ctx context.Context, buckets map[string]*minio.BucketMetadata) {
	for bucketName, b := range buckets {
		if _, err := m.GetBucketMetadata(ctx, bucketName); err == nil {
			continue // already imported
		}
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
			importLog.Warn("failed to save bucket metadata", "bucket", bucketName, "error", err)
		}
	}
	if len(buckets) > 0 {
		importLog.Info("imported buckets", "count", len(buckets))
	}
}

func (m *Manager) importObjectMetadata(ctx context.Context, objects map[string]map[string]*minio.ObjectMetadata) {
	count := 0
	for bucketName, objs := range objects {
		for objectKey, minioMeta := range objs {
			if _, err := m.GetObjectMetadata(ctx, bucketName, objectKey); err == nil {
				continue // already imported
			}
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
				importLog.Warn("failed to save object metadata", "bucket", bucketName, "key", objectKey, "error", err)
				continue
			}
			count++
		}
	}
	if count > 0 {
		importLog.Info("imported object metadata", "count", count)
	}
}

func (m *Manager) importGroups(ctx context.Context, groups map[string]*minio.ImportGroup, accessKeyToUUID map[string]uuid.UUID) {
	count := 0
	for groupName, g := range groups {
		if _, err := m.GetGroup(ctx, groupName); err == nil {
			continue // already imported
		}
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
				importLog.Warn("group member not found in imported users", "group", groupName, "access_key", accessKey)
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
			importLog.Warn("failed to save group", "group", groupName, "error", err)
			continue
		}
		count++
	}
	if count > 0 {
		importLog.Info("imported groups", "count", count)
	}
}

func (m *Manager) importServiceAccounts(ctx context.Context, sas map[string]*minio.ImportServiceAccount, accessKeyToUUID map[string]uuid.UUID) {
	count := 0
	for _, sa := range sas {
		if _, err := m.GetServiceAccount(ctx, sa.AccessKey); err == nil {
			continue // already imported
		}
		now := time.Now()
		updatedAt := sa.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		uid, ok := accessKeyToUUID[sa.ParentUser]
		if !ok {
			importLog.Warn("service account parent not found in imported users; skipping",
				"access_key", sa.AccessKey, "parent", sa.ParentUser)
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
			importLog.Warn("failed to save service account", "access_key", sa.AccessKey, "error", err)
			continue
		}
		count++
	}
	if count > 0 {
		importLog.Info("imported service accounts", "count", count)
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
		importLog.Warn("failed to parse session policy; falling back to inherit",
			"access_key", sa.AccessKey, "error", err)
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
	info, err := m.rootFS.Stat(consts.MinioMetadataDir)
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
		importLog.Warn("failed to parse bucket policy", "bucket", bucketName, "error", err)
		return nil
	}
	return &policyDoc
}
