package minio

import "time"

// BucketMetadata represents MinIO's bucket metadata (msgpack format)
type BucketMetadata struct {
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

// UserPolicy represents MinIO's policydb user policy
type UserPolicy struct {
	Version   int       `json:"version"`
	Policy    string    `json:"policy"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ObjectMetadata represents MinIO's fs.json format
type ObjectMetadata struct {
	Version  string                 `json:"version"`
	Checksum ChecksumInfo           `json:"checksum"`
	Meta     map[string]string      `json:"meta"`
}

// ChecksumInfo represents MinIO checksum information
type ChecksumInfo struct {
	Algorithm string   `json:"algorithm"`
	BlockSize int      `json:"blocksize"`
	Hashes    []string `json:"hashes"`
}

// User represents an imported MinIO user
type User struct {
	AccessKey string
	SecretKey string
	Status    string
	UpdatedAt time.Time
}

// Bucket represents an imported MinIO bucket
type Bucket struct {
	Name    string
	Owner   string
	Created time.Time
	Policy  string // S3 bucket policy JSON
}
