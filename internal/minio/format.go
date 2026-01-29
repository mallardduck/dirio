package minio

import (
	"encoding/json"
	"fmt"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
)

// FormatInfo represents MinIO's format.json structure
type FormatInfo struct {
	Version string        `json:"version"`
	Format  string        `json:"format"`
	ID      string        `json:"id"`
	FS      *FSFormatInfo `json:"fs,omitempty"`
	XL      *XLFormatInfo `json:"xl,omitempty"`
}

// FSFormatInfo represents filesystem mode format info
type FSFormatInfo struct {
	Version string `json:"version"`
}

// XLFormatInfo represents erasure coded mode format info
type XLFormatInfo struct {
	Version      string  `json:"version"`
	Distributed  bool    `json:"distributed"`
	DrivesPerSet int     `json:"drivesPerSet"`
	Sets         [][]int `json:"sets"`
}

// ValidateFormat checks if the MinIO data is in supported single-node FS mode.
// Returns an error if:
// - format.json doesn't exist
// - format is not "fs" (filesystem mode)
// - format is erasure coded or distributed
func ValidateFormat(minioFS billy.Filesystem) error {
	formatPath := "format.json"

	data, err := util.ReadFile(minioFS, formatPath)
	if err != nil {
		if isNotExist(err) {
			return fmt.Errorf("format.json not found - not a valid MinIO data directory")
		}
		return fmt.Errorf("failed to read format.json: %w", err)
	}

	var format FormatInfo
	if err := json.Unmarshal(data, &format); err != nil {
		return fmt.Errorf("failed to parse format.json: %w", err)
	}

	// Only support single-node filesystem mode
	if format.Format != "fs" {
		return fmt.Errorf("unsupported MinIO format: %s (only 'fs' single-node filesystem mode is supported)", format.Format)
	}

	// Ensure it's not distributed/erasure coded
	if format.XL != nil {
		return fmt.Errorf("erasure coded MinIO installations are not supported (detected XL format)")
	}

	// Validate FS format version
	if format.FS == nil {
		return fmt.Errorf("invalid fs format: missing fs field")
	}

	return nil
}
