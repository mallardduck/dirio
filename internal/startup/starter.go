// Package startup encapsulates the pre-server initialisation sequence that is
// shared between the serve and init commands.
//
// The three-phase design keeps concerns separate:
//
//   - [Init] is lightweight: directory creation, encryption keyring, root
//     filesystem, and DataConfig load-or-default.  Both commands call it.
//
//   - [Starter.MigrateMinIO] detects and imports existing MinIO data.  It is
//     a no-op when no .minio.sys directory is present.  Both serve and init
//     call it so that init can report the import to the operator and so that
//     subsequent serve runs see a consistent, authoritative DataConfig.
//
//   - [Starter.Prepare] is serve-only: it delegates to MigrateMinIO, then
//     persists a freshly-generated DataConfig when no existing or imported
//     config was found, and finally registers the global instance ID.  After
//     Prepare returns the Starter's components are ready to hand to server.New.
package startup

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/go-git/go-billy/v5"

	"github.com/mallardduck/dirio/internal/config/data"
	"github.com/mallardduck/dirio/internal/crypto"
	"github.com/mallardduck/dirio/internal/global"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/persistence/path"
)

// Starter holds the pre-server initialisation state.
type Starter struct {
	// DataDir is the resolved data directory path.
	DataDir string

	// DataConfig is the data configuration.
	// After Init it reflects whatever was on disk, or a fresh in-memory default.
	// After MigrateMinIO (or Prepare) it is authoritative.
	DataConfig *data.ConfigData

	rootFS  billy.Filesystem
	metaMgr *metadata.Manager

	// isNew is true when DataConfig was freshly generated rather than loaded
	// from an existing config.json on disk.  MigrateMinIO and Prepare may
	// set it to false when they write or reload the config.
	isNew bool
}

// RootFS returns the initialised root filesystem (available after Init).
func (s *Starter) RootFS() billy.Filesystem { return s.rootFS }

// MetadataManager returns the metadata manager, or nil if MigrateMinIO found
// no MinIO data and Prepare has not been called yet.
func (s *Starter) MetadataManager() *metadata.Manager { return s.metaMgr }

// IsNew reports whether the DataConfig was freshly created rather than loaded
// from an existing config.json.
func (s *Starter) IsNew() bool { return s.isNew }

// Close releases resources acquired by MigrateMinIO or Prepare (specifically
// the metadata bolt DB).  Safe to call when metaMgr is nil.  Should be called
// by the init command after it finishes; the serve command lets server.New take
// ownership and closes the manager during graceful shutdown instead.
func (s *Starter) Close() error {
	if s.metaMgr != nil {
		err := s.metaMgr.Close()
		s.metaMgr = nil
		return err
	}
	return nil
}

// Init creates a Starter by:
//  1. Ensuring the data directory exists (MkdirAll).
//  2. Initialising the encryption keyring.
//  3. Creating the root filesystem.
//  4. Loading DataConfig from .dirio/config.json, or generating a fresh
//     in-memory default (not yet persisted to disk).
//
// Shared between the serve and init commands.
func Init(dataDir string) (*Starter, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create data directory: %w", err)
	}

	if err := crypto.Init(dataDir); err != nil {
		return nil, fmt.Errorf("failed to initialise encryption: %w", err)
	}

	rootFS, err := path.NewRootFS(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create root filesystem: %w", err)
	}

	s := &Starter{
		DataDir: dataDir,
		rootFS:  rootFS,
	}

	if data.ConfigDataExists(rootFS) {
		dc, err := data.LoadDataConfig(rootFS)
		if err != nil {
			return nil, fmt.Errorf("failed to load data config: %w", err)
		}
		s.DataConfig = dc
	} else {
		s.DataConfig = data.DefaultDataConfig()
		s.isNew = true
	}

	return s, nil
}

// MigrateMinIO detects and imports existing MinIO data from .minio.sys.
//
// It is a fast no-op when no .minio.sys directory is present — no metadata
// manager is created in that case.  When MinIO data is found, it:
//  1. Creates the metadata manager (stored in the Starter for later use).
//  2. Runs CheckAndImportMinIO.
//  3. If the import wrote a new .dirio/config.json, reloads DataConfig so
//     the in-memory state reflects the real imported InstanceID and settings.
//
// This is safe to call regardless of whether Init loaded an existing config
// or created a fresh default — the reload always happens when the import ran,
// not just when the config was previously absent.  This fixes the edge case
// where "dirio init" creates a config.json before "dirio serve" runs the
// import, which would otherwise leave DataConfig pointing at the init-created
// config rather than the MinIO-sourced one.
//
// Both serve (via Prepare) and init call this method.
func (s *Starter) MigrateMinIO(ctx context.Context) error {
	log := logging.Component("startup")

	// Fast path: skip entirely when there is no MinIO data directory.
	if _, err := s.rootFS.Stat(path.MinIODir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Debug("minio data directory not found, skipping import")
			return nil
		}

		return fmt.Errorf("encountered additional error while finding minio dir for import: %w", err)
	}

	// MinIO data is present — we need the metadata manager.
	metaMgr, err := metadata.New(s.rootFS)
	if err != nil {
		return fmt.Errorf("failed to initialise metadata for minio import: %w", err)
	}
	s.metaMgr = metaMgr

	imported, err := metaMgr.CheckAndImportMinIO(ctx)
	if err != nil {
		log.Warn("minio data check & import failed", "error", err)
	}

	// Reload DataConfig whenever this call performed an import, regardless of
	// whether the config was fresh or pre-existing.  This is the key fix: when
	// "dirio init" has already written a config.json, s.isNew is false but the
	// import still overwrites config.json with MinIO-sourced settings.
	if imported {
		dc, err := data.LoadDataConfig(s.rootFS)
		if err != nil {
			return fmt.Errorf("failed to reload data config after minio import: %w", err)
		}
		s.DataConfig = dc
		s.isNew = false
		log.Info("data config loaded from minio import",
			"instance_id", dc.InstanceID,
			"region", dc.Region,
		)
	}

	return nil
}

// Prepare finalises the Starter for the serve command by:
//  1. Running MigrateMinIO (no-op if no .minio.sys is present).
//  2. Ensuring the metadata manager exists (creates it if MigrateMinIO
//     skipped because there was no MinIO data).
//  3. Persisting a freshly-generated DataConfig (with CLI region/credentials)
//     when neither an existing config nor a MinIO import was found.
//  4. Setting the global instance ID.
//
// After Prepare returns, DataConfig is authoritative, MetadataManager is
// ready to hand to server.New, and the global InstanceID has been set.
func (s *Starter) Prepare(ctx context.Context, region, accessKey, secretKey string, credentialsExplicit bool) error {
	log := logging.Component("startup")

	// Phase 1 — MinIO migration (shared logic, no-op when .minio.sys absent).
	if err := s.MigrateMinIO(ctx); err != nil {
		return err
	}

	// Phase 2 — Metadata manager: create it now if MigrateMinIO skipped it.
	if s.metaMgr == nil {
		metaMgr, err := metadata.New(s.rootFS)
		if err != nil {
			return fmt.Errorf("failed to initialise metadata: %w", err)
		}
		s.metaMgr = metaMgr
	}

	// Phase 3 — Persist a new config when no existing or imported config exists.
	if s.isNew {
		s.DataConfig.Region = region
		if credentialsExplicit {
			s.DataConfig.Credentials.AccessKey = accessKey
			s.DataConfig.Credentials.SecretKey = secretKey
			log.Info("persisting explicit CLI credentials to data config", "access_key", accessKey)
		} else {
			log.Info("no explicit credentials provided — data config created without credentials",
				"hint", `run "dirio init --access-key ... --secret-key ..." to configure admin credentials`,
			)
		}
		if err := data.SaveDataConfig(s.rootFS, s.DataConfig); err != nil {
			return fmt.Errorf("failed to save data config: %w", err)
		}
		s.isNew = false
		log.Info("initialised new data directory",
			"region", region,
			"compression", s.DataConfig.Compression.Enabled,
			"worm", s.DataConfig.WORMEnabled,
		)
	}

	// Phase 4 — DataConfig is now final; register the instance ID globally.
	global.SetGlobalInstanceID(s.DataConfig.InstanceID)
	log.Info("instance ID set", "instance_id", s.DataConfig.InstanceID)

	return nil
}
