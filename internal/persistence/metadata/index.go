package metadata

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	bbolt "go.etcd.io/bbolt"
)

// Bolt index bucket names.
const (
	boltBucketUsersByUsername  = "idx:users:by-username"
	boltBucketUsersByAccessKey = "idx:users:by-access-key"
)

// openDB opens the bbolt database at dbPath and ensures the two index buckets exist.
// The caller is responsible for calling db.Close() when done.
func openDB(dbPath string) (*bbolt.DB, error) {
	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt db: %w", err)
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(boltBucketUsersByUsername)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(boltBucketUsersByAccessKey)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create bolt buckets: %w", err)
	}

	return db, nil
}

// reconcileIndexes synchronises the bolt indexes against the JSON user files on disk.
// It must be called after buildUsersIndex so that usersIdx is populated.
//
// Phase A – remove stale entries (UUID not present in usersIdx).
// Phase B – add missing entries (UUID present but username/accessKey not indexed).
//
// Errors are logged and skipped (fail-soft) to match the pattern used elsewhere in this package.
func (m *Manager) reconcileIndexes(ctx context.Context) {
	m.usersMu.RLock()
	knownUUIDs := make(map[uuid.UUID]struct{}, len(m.usersIdx))
	for uid := range m.usersIdx {
		knownUUIDs[uid] = struct{}{}
	}
	m.usersMu.RUnlock()

	err := m.db.Update(func(tx *bbolt.Tx) error {
		byUsername := tx.Bucket([]byte(boltBucketUsersByUsername))
		byAccessKey := tx.Bucket([]byte(boltBucketUsersByAccessKey))

		// Phase A – collect and remove stale entries.
		stale := make([][]byte, 0)

		_ = byUsername.ForEach(func(k, v []byte) error {
			uid, err := uuid.FromBytes(v)
			if err != nil {
				stale = append(stale, append([]byte(nil), k...))
				return nil
			}
			if _, ok := knownUUIDs[uid]; !ok {
				stale = append(stale, append([]byte(nil), k...))
			}
			return nil
		})
		for _, k := range stale {
			_ = byUsername.Delete(k)
		}

		stale = stale[:0]
		_ = byAccessKey.ForEach(func(k, v []byte) error {
			uid, err := uuid.FromBytes(v)
			if err != nil {
				stale = append(stale, append([]byte(nil), k...))
				return nil
			}
			if _, ok := knownUUIDs[uid]; !ok {
				stale = append(stale, append([]byte(nil), k...))
			}
			return nil
		})
		for _, k := range stale {
			_ = byAccessKey.Delete(k)
		}

		// Phase B – add missing entries.
		for uid := range knownUUIDs {
			user, err := m.GetUser(ctx, uid)
			if err != nil {
				fmt.Printf("Warning: failed to load user %s during index reconciliation: %v\n", uid, err)
				continue
			}

			uidBytes := uid[:]

			if user.Username != "" {
				existing := byUsername.Get([]byte(user.Username))
				if existing == nil || !bytes.Equal(existing, uidBytes) {
					if err := byUsername.Put([]byte(user.Username), uidBytes); err != nil {
						return err
					}
				}
			}

			if user.AccessKey != "" {
				existing := byAccessKey.Get([]byte(user.AccessKey))
				if existing == nil || !bytes.Equal(existing, uidBytes) {
					if err := byAccessKey.Put([]byte(user.AccessKey), uidBytes); err != nil {
						return err
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Warning: failed to reconcile bolt indexes: %v\n", err)
	}
}

// GetUserByUsername retrieves a user by their username using the bolt index (O(1) on disk).
func (m *Manager) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	var userUID uuid.UUID
	err := m.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(boltBucketUsersByUsername))
		v := b.Get([]byte(username))
		if v == nil {
			return ErrUserNotFound
		}
		uid, err := uuid.FromBytes(v)
		if err != nil {
			return fmt.Errorf("corrupt index for username %q: %w", username, err)
		}
		userUID = uid
		return nil
	})
	if err != nil {
		return nil, err
	}

	return m.GetUser(ctx, userUID)
}

// GetUserByAccessKey retrieves a user by their access key using the bolt index (O(1) on disk).
func (m *Manager) GetUserByAccessKey(ctx context.Context, accessKey string) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	var userUID uuid.UUID
	err := m.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(boltBucketUsersByAccessKey))
		v := b.Get([]byte(accessKey))
		if v == nil {
			return ErrUserNotFound
		}
		uid, err := uuid.FromBytes(v)
		if err != nil {
			return fmt.Errorf("corrupt index for access key %q: %w", accessKey, err)
		}
		userUID = uid
		return nil
	})
	if err != nil {
		return nil, err
	}

	return m.GetUser(ctx, userUID)
}

// Close closes the underlying bbolt database.
func (m *Manager) Close() error {
	return m.db.Close()
}
