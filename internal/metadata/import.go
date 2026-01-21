package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

// ImportState tracks MinIO import status
type ImportState struct {
	Imported      bool      `json:"imported"`
	ImportedAt    time.Time `json:"importedAt"`
	MinIOModTime  time.Time `json:"minioModTime"`
	SourceVersion string    `json:"sourceVersion"`
}

// MinIOBucketMetadata represents MinIO's bucket metadata (msgpack format)
type MinIOBucketMetadata struct {
	Name                    string `msgpack:"Name"`
	Created                 []byte `msgpack:"Created"` // msgpack timestamp
	LockEnabled             bool   `msgpack:"LockEnabled"`
	PolicyConfigJSON        string `msgpack:"PolicyConfigJSON"`
	VersioningConfigXML     string `msgpack:"VersioningConfigXML"`
	NotificationConfigXML   string `msgpack:"NotificationConfigXML"`
	LifecycleConfigXML      string `msgpack:"LifecycleConfigXML"`
	ObjectLockConfigXML     string `msgpack:"ObjectLockConfigXML"`
	EncryptionConfigXML     string `msgpack:"EncryptionConfigXML"`
	TaggingConfigXML        string `msgpack:"TaggingConfigXML"`
	QuotaConfigJSON         string `msgpack:"QuotaConfigJSON"`
	ReplicationConfigXML    string `msgpack:"ReplicationConfigXML"`
	BucketTargetsConfigJSON string `msgpack:"BucketTargetsConfigJSON"`
}

// MinIOUserIdentity represents MinIO's user identity.json format
type MinIOUserIdentity struct {
	Version     int                     `json:"version"`
	Credentials MinIOUserCredentials    `json:"credentials"`
	UpdatedAt   time.Time               `json:"updatedAt"`
}

type MinIOUserCredentials struct {
	AccessKey  string    `json:"accessKey"`
	SecretKey  string    `json:"secretKey"`
	Expiration time.Time `json:"expiration"`
	Status     string    `json:"status"`
}

// MinIOUserPolicy represents MinIO's policydb user policy
type MinIOUserPolicy struct {
	Version   int       `json:"version"`
	Policy    string    `json:"policy"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// MinIOObjectMetadata represents MinIO's fs.json format
type MinIOObjectMetadata struct {
	Version  string                     `json:"version"`
	Checksum MinIOChecksumInfo          `json:"checksum"`
	Meta     map[string]string          `json:"meta"`
}

type MinIOChecksumInfo struct {
	Algorithm string   `json:"algorithm"`
	BlockSize int      `json:"blocksize"`
	Hashes    []string `json:"hashes"`
}

// CheckAndImportMinIO checks for MinIO data and imports if needed
func (m *Manager) CheckAndImportMinIO() error {
	// Check if .minio.sys exists
	if _, err := os.Stat(m.minioSysDir); os.IsNotExist(err) {
		return nil // No MinIO data to import
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

	// Perform import
	fmt.Println("Detected MinIO data. Starting import...")
	
	if err := m.importMinIOUsers(); err != nil {
		return fmt.Errorf("failed to import users: %w", err)
	}

	if err := m.importMinIOBuckets(); err != nil {
		return fmt.Errorf("failed to import buckets: %w", err)
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

// importMinIOUsers imports users from MinIO IAM
func (m *Manager) importMinIOUsers() error {
	usersDir := filepath.Join(m.minioSysDir, "config", "iam", "users")
	
	if _, err := os.Stat(usersDir); os.IsNotExist(err) {
		return nil // No users to import
	}

	entries, err := os.ReadDir(usersDir)
	if err != nil {
		return err
	}

	users := make(map[string]*User)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		username := entry.Name()
		identityPath := filepath.Join(usersDir, username, "identity.json")
		
		// Read identity.json
		data, err := os.ReadFile(identityPath)
		if err != nil {
			fmt.Printf("Warning: failed to read identity for user %s: %v\n", username, err)
			continue
		}

		var identity MinIOUserIdentity
		if err := json.Unmarshal(data, &identity); err != nil {
			fmt.Printf("Warning: failed to parse identity for user %s: %v\n", username, err)
			continue
		}

		// Import user
		users[username] = &User{
			AccessKey: identity.Credentials.AccessKey,
			SecretKey: identity.Credentials.SecretKey,
			Status:    identity.Credentials.Status,
			UpdatedAt: identity.UpdatedAt,
		}

		fmt.Printf("Imported user: %s\n", username)
	}

	// Save users
	return m.SaveUsers(users)
}

// importMinIOBuckets imports bucket metadata from MinIO
func (m *Manager) importMinIOBuckets() error {
	bucketsMetaDir := filepath.Join(m.minioSysDir, "buckets")
	
	if _, err := os.Stat(bucketsMetaDir); os.IsNotExist(err) {
		return nil // No buckets to import
	}

	entries, err := os.ReadDir(bucketsMetaDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == ".minio.sys" {
			continue
		}

		bucketName := entry.Name()
		metadataPath := filepath.Join(bucketsMetaDir, bucketName, ".metadata.bin")
		
		// Check if metadata file exists
		if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
			// Create basic metadata
			meta := &BucketMetadata{
				Name:    bucketName,
				Owner:   "root",
				Created: time.Now(),
			}
			if err := m.saveBucketMetadata(bucketName, meta); err != nil {
				fmt.Printf("Warning: failed to create metadata for bucket %s: %v\n", bucketName, err)
			}
			continue
		}

		// Read and parse MinIO metadata
		data, err := os.ReadFile(metadataPath)
		if err != nil {
			fmt.Printf("Warning: failed to read metadata for bucket %s: %v\n", bucketName, err)
			continue
		}

		var minioMeta MinIOBucketMetadata
		if err := msgpack.Unmarshal(data, &minioMeta); err != nil {
			fmt.Printf("Warning: failed to parse metadata for bucket %s: %v\n", bucketName, err)
			continue
		}

		// Convert to our format
		meta := &BucketMetadata{
			Name:    minioMeta.Name,
			Owner:   "root", // MinIO doesn't store owner in bucket metadata
			Created: time.Now(), // TODO: Parse Created timestamp
			Policy:  minioMeta.PolicyConfigJSON,
		}

		if err := m.saveBucketMetadata(bucketName, meta); err != nil {
			fmt.Printf("Warning: failed to save metadata for bucket %s: %v\n", bucketName, err)
			continue
		}

		fmt.Printf("Imported bucket: %s\n", bucketName)
	}

	return nil
}

// getImportState retrieves the import state
func (m *Manager) getImportState() (*ImportState, error) {
	statePath := filepath.Join(m.metadataDir, ".import-state")
	
	data, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return &ImportState{}, nil
	}
	if err != nil {
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
	statePath := filepath.Join(m.metadataDir, ".import-state")
	
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statePath, data, 0644)
}

// getMinIOModTime gets the last modification time of MinIO data
func (m *Manager) getMinIOModTime() (time.Time, error) {
	info, err := os.Stat(m.minioSysDir)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
