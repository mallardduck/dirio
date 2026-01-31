package minio

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/tinylib/msgp/msgp"
)

const (
	// MinIO .metadata.bin format constants
	bucketMetadataFormat  = 1
	bucketMetadataVersion = 1
)

// parseBucketMetadataBin parses a MinIO .metadata.bin file and returns BucketMetadata.
// The file format is:
//   - 4-byte header: [format:uint16][version:uint16] (little endian)
//   - msgpack-encoded map with bucket metadata fields
//
// This parser supports format=1, version=1 from MinIO RELEASE.2022-10-24T18-35-07Z.
// It will fail on unknown formats and warn on unexpected versions.
func parseBucketMetadataBin(data []byte) (*BucketMetadata, error) {
	if len(data) <= 4 {
		return nil, fmt.Errorf("metadata.bin too small: %d bytes", len(data))
	}

	// Read and validate header
	format := binary.LittleEndian.Uint16(data[0:2])
	version := binary.LittleEndian.Uint16(data[2:4])

	if format != bucketMetadataFormat {
		return nil, fmt.Errorf("unsupported metadata format %d (expected %d)", format, bucketMetadataFormat)
	}

	if version != bucketMetadataVersion {
		// Warn but try to continue - we might still be able to read the map
		fmt.Printf("WARNING: unexpected metadata version %d (expected %d), attempting to parse anyway\n", version, bucketMetadataVersion)
	}

	// Create msgpack reader for the body (skip 4-byte header)
	reader := msgp.NewReader(bytes.NewReader(data[4:]))

	// Read map header
	mapSize, err := reader.ReadMapHeader()
	if err != nil {
		return nil, fmt.Errorf("failed to read map header: %w", err)
	}

	meta := &BucketMetadata{}

	// Read all map entries
	for i := uint32(0); i < mapSize; i++ {
		// Read field name
		fieldName, err := reader.ReadString()
		if err != nil {
			return nil, fmt.Errorf("failed to read field name at index %d: %w", i, err)
		}

		// Read field value based on name
		switch fieldName {
		case "Name":
			meta.Name, err = reader.ReadString()
			if err != nil {
				return nil, fmt.Errorf("failed to read Name: %w", err)
			}

		case "Created":
			meta.Created, err = reader.ReadTime()
			if err != nil {
				return nil, fmt.Errorf("failed to read Created: %w", err)
			}

		case "LockEnabled":
			meta.LockEnabled, err = reader.ReadBool()
			if err != nil {
				return nil, fmt.Errorf("failed to read LockEnabled: %w", err)
			}

		case "PolicyConfigJSON":
			meta.PolicyConfigJSON, err = reader.ReadBytes(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to read PolicyConfigJSON: %w", err)
			}

		case "NotificationConfigXML":
			meta.NotificationConfigXML, err = reader.ReadBytes(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to read NotificationConfigXML: %w", err)
			}

		case "LifecycleConfigXML":
			meta.LifecycleConfigXML, err = reader.ReadBytes(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to read LifecycleConfigXML: %w", err)
			}

		case "ObjectLockConfigXML":
			meta.ObjectLockConfigXML, err = reader.ReadBytes(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to read ObjectLockConfigXML: %w", err)
			}

		case "VersioningConfigXML":
			meta.VersioningConfigXML, err = reader.ReadBytes(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to read VersioningConfigXML: %w", err)
			}

		case "EncryptionConfigXML":
			meta.EncryptionConfigXML, err = reader.ReadBytes(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to read EncryptionConfigXML: %w", err)
			}

		case "TaggingConfigXML":
			meta.TaggingConfigXML, err = reader.ReadBytes(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to read TaggingConfigXML: %w", err)
			}

		case "QuotaConfigJSON":
			meta.QuotaConfigJSON, err = reader.ReadBytes(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to read QuotaConfigJSON: %w", err)
			}

		case "ReplicationConfigXML":
			meta.ReplicationConfigXML, err = reader.ReadBytes(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to read ReplicationConfigXML: %w", err)
			}

		case "BucketTargetsConfigJSON":
			meta.BucketTargetsConfigJSON, err = reader.ReadBytes(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to read BucketTargetsConfigJSON: %w", err)
			}

		case "BucketTargetsConfigMetaJSON":
			meta.BucketTargetsConfigMetaJSON, err = reader.ReadBytes(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to read BucketTargetsConfigMetaJSON: %w", err)
			}

		case "PolicyConfigUpdatedAt":
			meta.PolicyConfigUpdatedAt, err = reader.ReadTime()
			if err != nil {
				return nil, fmt.Errorf("failed to read PolicyConfigUpdatedAt: %w", err)
			}

		case "ObjectLockConfigUpdatedAt":
			meta.ObjectLockConfigUpdatedAt, err = reader.ReadTime()
			if err != nil {
				return nil, fmt.Errorf("failed to read ObjectLockConfigUpdatedAt: %w", err)
			}

		case "EncryptionConfigUpdatedAt":
			meta.EncryptionConfigUpdatedAt, err = reader.ReadTime()
			if err != nil {
				return nil, fmt.Errorf("failed to read EncryptionConfigUpdatedAt: %w", err)
			}

		case "TaggingConfigUpdatedAt":
			meta.TaggingConfigUpdatedAt, err = reader.ReadTime()
			if err != nil {
				return nil, fmt.Errorf("failed to read TaggingConfigUpdatedAt: %w", err)
			}

		case "QuotaConfigUpdatedAt":
			meta.QuotaConfigUpdatedAt, err = reader.ReadTime()
			if err != nil {
				return nil, fmt.Errorf("failed to read QuotaConfigUpdatedAt: %w", err)
			}

		case "ReplicationConfigUpdatedAt":
			meta.ReplicationConfigUpdatedAt, err = reader.ReadTime()
			if err != nil {
				return nil, fmt.Errorf("failed to read ReplicationConfigUpdatedAt: %w", err)
			}

		case "VersioningConfigUpdatedAt":
			meta.VersioningConfigUpdatedAt, err = reader.ReadTime()
			if err != nil {
				return nil, fmt.Errorf("failed to read VersioningConfigUpdatedAt: %w", err)
			}

		default:
			// Unknown field - skip it for forward compatibility
			err = reader.Skip()
			if err != nil {
				return nil, fmt.Errorf("failed to skip unknown field %s: %w", fieldName, err)
			}
		}
	}

	return meta, nil
}

// mergeBucketMetadata merges binary metadata into legacy metadata.
// Legacy config takes precedence - binary metadata only fills in missing fields.
// This handles hybrid configurations where some data is in legacy files and some in .metadata.bin.
func mergeBucketMetadata(legacy *BucketMetadata, binary *BucketMetadata) {
	// Only use binary data if legacy doesn't have it

	// Name - keep legacy (always present)
	// Created - use binary if legacy is zero
	if legacy.Created.IsZero() && !binary.Created.IsZero() {
		legacy.Created = binary.Created
	}

	// LockEnabled - legacy takes precedence
	if !legacy.LockEnabled && binary.LockEnabled {
		legacy.LockEnabled = binary.LockEnabled
	}

	// Config blobs - only use binary if legacy is empty
	if len(legacy.PolicyConfigJSON) == 0 && len(binary.PolicyConfigJSON) > 0 {
		legacy.PolicyConfigJSON = binary.PolicyConfigJSON
		legacy.PolicyConfigUpdatedAt = binary.PolicyConfigUpdatedAt
	}

	if len(legacy.NotificationConfigXML) == 0 && len(binary.NotificationConfigXML) > 0 {
		legacy.NotificationConfigXML = binary.NotificationConfigXML
	}

	if len(legacy.LifecycleConfigXML) == 0 && len(binary.LifecycleConfigXML) > 0 {
		legacy.LifecycleConfigXML = binary.LifecycleConfigXML
	}

	if len(legacy.ObjectLockConfigXML) == 0 && len(binary.ObjectLockConfigXML) > 0 {
		legacy.ObjectLockConfigXML = binary.ObjectLockConfigXML
		legacy.ObjectLockConfigUpdatedAt = binary.ObjectLockConfigUpdatedAt
	}

	if len(legacy.VersioningConfigXML) == 0 && len(binary.VersioningConfigXML) > 0 {
		legacy.VersioningConfigXML = binary.VersioningConfigXML
		legacy.VersioningConfigUpdatedAt = binary.VersioningConfigUpdatedAt
	}

	if len(legacy.EncryptionConfigXML) == 0 && len(binary.EncryptionConfigXML) > 0 {
		legacy.EncryptionConfigXML = binary.EncryptionConfigXML
		legacy.EncryptionConfigUpdatedAt = binary.EncryptionConfigUpdatedAt
	}

	if len(legacy.TaggingConfigXML) == 0 && len(binary.TaggingConfigXML) > 0 {
		legacy.TaggingConfigXML = binary.TaggingConfigXML
		legacy.TaggingConfigUpdatedAt = binary.TaggingConfigUpdatedAt
	}

	if len(legacy.QuotaConfigJSON) == 0 && len(binary.QuotaConfigJSON) > 0 {
		legacy.QuotaConfigJSON = binary.QuotaConfigJSON
		legacy.QuotaConfigUpdatedAt = binary.QuotaConfigUpdatedAt
	}

	if len(legacy.ReplicationConfigXML) == 0 && len(binary.ReplicationConfigXML) > 0 {
		legacy.ReplicationConfigXML = binary.ReplicationConfigXML
		legacy.ReplicationConfigUpdatedAt = binary.ReplicationConfigUpdatedAt
	}

	if len(legacy.BucketTargetsConfigJSON) == 0 && len(binary.BucketTargetsConfigJSON) > 0 {
		legacy.BucketTargetsConfigJSON = binary.BucketTargetsConfigJSON
	}

	if len(legacy.BucketTargetsConfigMetaJSON) == 0 && len(binary.BucketTargetsConfigMetaJSON) > 0 {
		legacy.BucketTargetsConfigMetaJSON = binary.BucketTargetsConfigMetaJSON
	}
}
