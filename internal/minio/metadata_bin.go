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
	for i := range mapSize {
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
func mergeBucketMetadata(legacyMeta, binaryMeta *BucketMetadata) {
	// Only use binaryMeta data if legacyMeta doesn't have it

	// Name - keep legacyMeta (always present)
	// Created - use binaryMeta if legacyMeta is zero
	if legacyMeta.Created.IsZero() && !binaryMeta.Created.IsZero() {
		legacyMeta.Created = binaryMeta.Created
	}

	// LockEnabled - legacyMeta takes precedence
	if !legacyMeta.LockEnabled && binaryMeta.LockEnabled {
		legacyMeta.LockEnabled = binaryMeta.LockEnabled
	}

	// Config blobs - only use binaryMeta if legacyMeta is empty
	if len(legacyMeta.PolicyConfigJSON) == 0 && len(binaryMeta.PolicyConfigJSON) > 0 {
		legacyMeta.PolicyConfigJSON = binaryMeta.PolicyConfigJSON
		legacyMeta.PolicyConfigUpdatedAt = binaryMeta.PolicyConfigUpdatedAt
	}

	if len(legacyMeta.NotificationConfigXML) == 0 && len(binaryMeta.NotificationConfigXML) > 0 {
		legacyMeta.NotificationConfigXML = binaryMeta.NotificationConfigXML
	}

	if len(legacyMeta.LifecycleConfigXML) == 0 && len(binaryMeta.LifecycleConfigXML) > 0 {
		legacyMeta.LifecycleConfigXML = binaryMeta.LifecycleConfigXML
	}

	if len(legacyMeta.ObjectLockConfigXML) == 0 && len(binaryMeta.ObjectLockConfigXML) > 0 {
		legacyMeta.ObjectLockConfigXML = binaryMeta.ObjectLockConfigXML
		legacyMeta.ObjectLockConfigUpdatedAt = binaryMeta.ObjectLockConfigUpdatedAt
	}

	if len(legacyMeta.VersioningConfigXML) == 0 && len(binaryMeta.VersioningConfigXML) > 0 {
		legacyMeta.VersioningConfigXML = binaryMeta.VersioningConfigXML
		legacyMeta.VersioningConfigUpdatedAt = binaryMeta.VersioningConfigUpdatedAt
	}

	if len(legacyMeta.EncryptionConfigXML) == 0 && len(binaryMeta.EncryptionConfigXML) > 0 {
		legacyMeta.EncryptionConfigXML = binaryMeta.EncryptionConfigXML
		legacyMeta.EncryptionConfigUpdatedAt = binaryMeta.EncryptionConfigUpdatedAt
	}

	if len(legacyMeta.TaggingConfigXML) == 0 && len(binaryMeta.TaggingConfigXML) > 0 {
		legacyMeta.TaggingConfigXML = binaryMeta.TaggingConfigXML
		legacyMeta.TaggingConfigUpdatedAt = binaryMeta.TaggingConfigUpdatedAt
	}

	if len(legacyMeta.QuotaConfigJSON) == 0 && len(binaryMeta.QuotaConfigJSON) > 0 {
		legacyMeta.QuotaConfigJSON = binaryMeta.QuotaConfigJSON
		legacyMeta.QuotaConfigUpdatedAt = binaryMeta.QuotaConfigUpdatedAt
	}

	if len(legacyMeta.ReplicationConfigXML) == 0 && len(binaryMeta.ReplicationConfigXML) > 0 {
		legacyMeta.ReplicationConfigXML = binaryMeta.ReplicationConfigXML
		legacyMeta.ReplicationConfigUpdatedAt = binaryMeta.ReplicationConfigUpdatedAt
	}

	if len(legacyMeta.BucketTargetsConfigJSON) == 0 && len(binaryMeta.BucketTargetsConfigJSON) > 0 {
		legacyMeta.BucketTargetsConfigJSON = binaryMeta.BucketTargetsConfigJSON
	}

	if len(legacyMeta.BucketTargetsConfigMetaJSON) == 0 && len(binaryMeta.BucketTargetsConfigMetaJSON) > 0 {
		legacyMeta.BucketTargetsConfigMetaJSON = binaryMeta.BucketTargetsConfigMetaJSON
	}
}
