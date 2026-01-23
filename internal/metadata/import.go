package metadata

import (
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
func (m *Manager) CheckAndImportMinIO() error {
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
			policy := &Policy{
				Name:       minioPolicy.Name,
				PolicyJSON: minioPolicy.PolicyJSON,
				CreateDate: minioPolicy.CreateDate,
				UpdateDate: minioPolicy.UpdateDate,
			}
			if err := m.SavePolicy(policy); err != nil {
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
				AccessKey:      minioUser.AccessKey,
				SecretKey:      minioUser.SecretKey,
				Status:         minioUser.Status,
				UpdatedAt:      minioUser.UpdatedAt,
				AttachedPolicy: minioUser.AttachedPolicy,
			}
		}
		if err := m.SaveUsers(dirioUsers); err != nil {
			return fmt.Errorf("failed to save imported users: %w", err)
		}
		fmt.Printf("Imported %d users\n", len(dirioUsers))
	}

	// Convert and save buckets
	if len(result.Buckets) > 0 {
		for bucketName, minioBucket := range result.Buckets {
			meta := &BucketMetadata{
				Name:    minioBucket.Name,
				Owner:   minioBucket.Owner,
				Created: minioBucket.Created,
				Policy:  minioBucket.Policy,
			}
			if err := m.saveBucketMetadata(bucketName, meta); err != nil {
				fmt.Printf("Warning: failed to save metadata for bucket %s: %v\n", bucketName, err)
				continue
			}
		}
		fmt.Printf("Imported %d buckets\n", len(result.Buckets))
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
	data, err := json.MarshalIndent(state, "", "  ")
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
