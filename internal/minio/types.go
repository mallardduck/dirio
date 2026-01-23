package minio

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"time"
)

// BucketMetadata contains bucket metadata.
// When adding/removing fields, regenerate the marshal code using the go generate above.
// Only changing meaning of fields requires a version bump.
// bucketMetadataFormat refers to the format.
// bucketMetadataVersion can be used to track a rolling upgrade of a field.
type BucketMetadata struct {
	Name                        string
	Created                     time.Time
	LockEnabled                 bool // legacy not used anymore.
	PolicyConfigJSON            []byte
	NotificationConfigXML       []byte
	LifecycleConfigXML          []byte
	ObjectLockConfigXML         []byte
	VersioningConfigXML         []byte
	EncryptionConfigXML         []byte
	TaggingConfigXML            []byte
	QuotaConfigJSON             []byte
	ReplicationConfigXML        []byte
	BucketTargetsConfigJSON     []byte
	BucketTargetsConfigMetaJSON []byte
	PolicyConfigUpdatedAt       time.Time
	ObjectLockConfigUpdatedAt   time.Time
	EncryptionConfigUpdatedAt   time.Time
	TaggingConfigUpdatedAt      time.Time
	QuotaConfigUpdatedAt        time.Time
	ReplicationConfigUpdatedAt  time.Time
	VersioningConfigUpdatedAt   time.Time
}

// Example legacy config file names (adjust paths if needed)
var legacyConfigFiles = []string{
	"policy.json",
	"notification.xml",
	"lifecycle.xml",
	"encryption.xml",
	"tagging.xml",
	"quota.json",
	"replication.xml",
	"bucket-targets.json",
	"object-lock-enabled.json",
}

// readLegacyBucketMetadata reads FS-mode legacy bucket metadata
// and populates the exported fields in BucketMetadata.
// `readFileFunc` should return []byte for a given file path, similar to os.ReadFile.
func readLegacyBucketMetadata(bucketName string, readFileFunc func(string) ([]byte, error)) (*BucketMetadata, error) {
	b := &BucketMetadata{
		Name:    bucketName,
		Created: time.Now(),
	}

	for _, file := range legacyConfigFiles {
		filePath := path.Join(".minio.sys/buckets", bucketName, file)
		data, err := readFileFunc(filePath)
		if err != nil {
			// skip missing files
			var pathErr *os.PathError
			if errors.As(err, &pathErr) {
				continue
			}
			return nil, fmt.Errorf("failed to read legacy config %s: %w", file, err)
		}

		switch file {
		case "policy.json":
			b.PolicyConfigJSON = data
			if b.PolicyConfigUpdatedAt.IsZero() {
				b.PolicyConfigUpdatedAt = b.Created
			}
		case "notification.xml":
			b.NotificationConfigXML = data
		case "lifecycle.xml":
			b.LifecycleConfigXML = data
		case "object-lock-enabled.json":
			// legacy boolean flag
			if bytes.Equal(data, []byte(`{"x-amz-bucket-object-lock-enabled":true}`)) {
				b.ObjectLockConfigXML = []byte(`<ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ObjectLockEnabled>Enabled</ObjectLockEnabled></ObjectLockConfiguration>`)
				b.VersioningConfigXML = []byte(`<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`)
				b.LockEnabled = false
			}
		case "encryption.xml":
			b.EncryptionConfigXML = data
			if b.EncryptionConfigUpdatedAt.IsZero() {
				b.EncryptionConfigUpdatedAt = b.Created
			}
		case "tagging.xml":
			b.TaggingConfigXML = data
			if b.TaggingConfigUpdatedAt.IsZero() {
				b.TaggingConfigUpdatedAt = b.Created
			}
		case "quota.json":
			b.QuotaConfigJSON = data
			if b.QuotaConfigUpdatedAt.IsZero() {
				b.QuotaConfigUpdatedAt = b.Created
			}
		case "replication.xml":
			b.ReplicationConfigXML = data
			if b.ReplicationConfigUpdatedAt.IsZero() {
				b.ReplicationConfigUpdatedAt = b.Created
			}
		case "bucket-targets.json":
			b.BucketTargetsConfigJSON = data
			// optionally parse metadata JSON if present
			metaFile := path.Join(".minio.sys/buckets", bucketName, "bucket-targets-meta.json")
			metaBytes, _ := readFileFunc(metaFile)
			b.BucketTargetsConfigMetaJSON = metaBytes
		}
	}

	return b, nil
}

// UserIdentity represents MinIO's user identity.json format
type UserIdentity struct {
	Version     int             `json:"version"`
	Credentials UserCredentials `json:"credentials"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// UserCredentials represents MinIO user credentials
type UserCredentials struct {
	AccessKey  string    `json:"accessKey"`
	SecretKey  string    `json:"secretKey"`
	Expiration time.Time `json:"expiration"`
	Status     string    `json:"status"`
}

// UserPolicyMapping represents MinIO's policydb user policy mapping
type UserPolicyMapping struct {
	Version   int       `json:"version"`
	Policy    string    `json:"policy"` // Name of the attached policy
	UpdatedAt time.Time `json:"updatedAt"`
}

// PolicyFile represents MinIO's IAM policy file format
type PolicyFile struct {
	Version    int                    `json:"Version"`
	Policy     map[string]interface{} `json:"Policy"` // The IAM policy document
	CreateDate time.Time              `json:"CreateDate"`
	UpdateDate time.Time              `json:"UpdateDate"`
}

// ObjectMetadata represents MinIO's fs.json format
type ObjectMetadata struct {
	Version  string            `json:"version"`
	Checksum ChecksumInfo      `json:"checksum"`
	Meta     map[string]string `json:"meta"`
}

// ChecksumInfo represents MinIO checksum information
type ChecksumInfo struct {
	Algorithm string   `json:"algorithm"`
	BlockSize int      `json:"blocksize"`
	Hashes    []string `json:"hashes"`
}

// User represents an imported MinIO user
type User struct {
	AccessKey      string
	SecretKey      string
	Status         string
	UpdatedAt      time.Time
	AttachedPolicy string // Policy name attached to this user (from policydb)
}

// Bucket represents an imported MinIO bucket
type Bucket struct {
	Name    string
	Owner   string
	Created time.Time
	Policy  string // S3 bucket policy JSON
}

// Policy represents a MinIO IAM policy
type Policy struct {
	Name       string
	PolicyJSON string // The actual IAM policy JSON (S3 format)
	CreateDate time.Time
	UpdateDate time.Time
}
