package minioimport

import (
	"fmt"

	"github.com/tinylib/msgp/msgp"
)

// bucketMetadataFieldReader is a function that reads one msgpack field into BucketMetadata.
type bucketMetadataFieldReader func(meta *BucketMetadata, r *msgp.Reader) error

// bucketMetadataFields maps each known .metadata.bin field name to its reader function.
// Fields not present in the map are skipped for forward compatibility.
var bucketMetadataFields = map[string]bucketMetadataFieldReader{
	"Name": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadString()
		if err != nil {
			return fmt.Errorf("failed to read Name: %w", err)
		}
		m.Name = v
		return nil
	},
	"Created": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadTime()
		if err != nil {
			return fmt.Errorf("failed to read Created: %w", err)
		}
		m.Created = v
		return nil
	},
	"LockEnabled": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBool()
		if err != nil {
			return fmt.Errorf("failed to read LockEnabled: %w", err)
		}
		m.LockEnabled = v
		return nil
	},
	"PolicyConfigJSON": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBytes(nil)
		if err != nil {
			return fmt.Errorf("failed to read PolicyConfigJSON: %w", err)
		}
		m.PolicyConfigJSON = v
		return nil
	},
	"NotificationConfigXML": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBytes(nil)
		if err != nil {
			return fmt.Errorf("failed to read NotificationConfigXML: %w", err)
		}
		m.NotificationConfigXML = v
		return nil
	},
	"LifecycleConfigXML": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBytes(nil)
		if err != nil {
			return fmt.Errorf("failed to read LifecycleConfigXML: %w", err)
		}
		m.LifecycleConfigXML = v
		return nil
	},
	"ObjectLockConfigXML": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBytes(nil)
		if err != nil {
			return fmt.Errorf("failed to read ObjectLockConfigXML: %w", err)
		}
		m.ObjectLockConfigXML = v
		return nil
	},
	"VersioningConfigXML": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBytes(nil)
		if err != nil {
			return fmt.Errorf("failed to read VersioningConfigXML: %w", err)
		}
		m.VersioningConfigXML = v
		return nil
	},
	"EncryptionConfigXML": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBytes(nil)
		if err != nil {
			return fmt.Errorf("failed to read EncryptionConfigXML: %w", err)
		}
		m.EncryptionConfigXML = v
		return nil
	},
	"TaggingConfigXML": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBytes(nil)
		if err != nil {
			return fmt.Errorf("failed to read TaggingConfigXML: %w", err)
		}
		m.TaggingConfigXML = v
		return nil
	},
	"QuotaConfigJSON": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBytes(nil)
		if err != nil {
			return fmt.Errorf("failed to read QuotaConfigJSON: %w", err)
		}
		m.QuotaConfigJSON = v
		return nil
	},
	"ReplicationConfigXML": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBytes(nil)
		if err != nil {
			return fmt.Errorf("failed to read ReplicationConfigXML: %w", err)
		}
		m.ReplicationConfigXML = v
		return nil
	},
	"BucketTargetsConfigJSON": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBytes(nil)
		if err != nil {
			return fmt.Errorf("failed to read BucketTargetsConfigJSON: %w", err)
		}
		m.BucketTargetsConfigJSON = v
		return nil
	},
	"BucketTargetsConfigMetaJSON": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadBytes(nil)
		if err != nil {
			return fmt.Errorf("failed to read BucketTargetsConfigMetaJSON: %w", err)
		}
		m.BucketTargetsConfigMetaJSON = v
		return nil
	},
	"PolicyConfigUpdatedAt": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadTime()
		if err != nil {
			return fmt.Errorf("failed to read PolicyConfigUpdatedAt: %w", err)
		}
		m.PolicyConfigUpdatedAt = v
		return nil
	},
	"ObjectLockConfigUpdatedAt": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadTime()
		if err != nil {
			return fmt.Errorf("failed to read ObjectLockConfigUpdatedAt: %w", err)
		}
		m.ObjectLockConfigUpdatedAt = v
		return nil
	},
	"EncryptionConfigUpdatedAt": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadTime()
		if err != nil {
			return fmt.Errorf("failed to read EncryptionConfigUpdatedAt: %w", err)
		}
		m.EncryptionConfigUpdatedAt = v
		return nil
	},
	"TaggingConfigUpdatedAt": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadTime()
		if err != nil {
			return fmt.Errorf("failed to read TaggingConfigUpdatedAt: %w", err)
		}
		m.TaggingConfigUpdatedAt = v
		return nil
	},
	"QuotaConfigUpdatedAt": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadTime()
		if err != nil {
			return fmt.Errorf("failed to read QuotaConfigUpdatedAt: %w", err)
		}
		m.QuotaConfigUpdatedAt = v
		return nil
	},
	"ReplicationConfigUpdatedAt": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadTime()
		if err != nil {
			return fmt.Errorf("failed to read ReplicationConfigUpdatedAt: %w", err)
		}
		m.ReplicationConfigUpdatedAt = v
		return nil
	},
	"VersioningConfigUpdatedAt": func(m *BucketMetadata, r *msgp.Reader) error {
		v, err := r.ReadTime()
		if err != nil {
			return fmt.Errorf("failed to read VersioningConfigUpdatedAt: %w", err)
		}
		m.VersioningConfigUpdatedAt = v
		return nil
	},
}
