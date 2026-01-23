package hostname

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const idBytes = 3 // 6 hex chars

// readMachineID attempts to read a stable machine identifier.
// Returns a short hex string or empty string on failure.
// Supports Linux, macOS, and Windows.
func readMachineID() string {
	switch runtime.GOOS {
	case "linux":
		return readLinuxMachineID()
	case "darwin":
		return readDarwinMachineID()
	case "windows":
		return readWindowsMachineID()
	default:
		return ""
	}
}

// readLinuxMachineID reads /etc/machine-id on Linux systems.
func readLinuxMachineID() string {
	data, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		// Try alternative location
		data, err = os.ReadFile("/var/lib/dbus/machine-id")
		if err != nil {
			return ""
		}
	}

	id := strings.TrimSpace(string(data))
	if len(id) < idBytes*2 {
		return ""
	}

	raw, err := hex.DecodeString(id[:idBytes*2])
	if err != nil {
		return ""
	}

	return hex.EncodeToString(raw)
}

// readDarwinMachineID uses IOPlatformUUID on macOS systems.
func readDarwinMachineID() string {
	cmd := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Look for IOPlatformUUID in the output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "IOPlatformUUID") {
			// Extract UUID from line like: "IOPlatformUUID" = "XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX"
			parts := strings.Split(line, "\"")
			if len(parts) >= 4 {
				uuid := strings.ReplaceAll(parts[3], "-", "")
				return hashAndTruncate(uuid)
			}
		}
	}

	return ""
}

// readWindowsMachineID uses the MachineGuid from the Windows registry.
func readWindowsMachineID() string {
	cmd := exec.Command("reg", "query", "HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Cryptography", "/v", "MachineGuid")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse output like: "MachineGuid    REG_SZ    XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "MachineGuid") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				guid := strings.ReplaceAll(parts[len(parts)-1], "-", "")
				return hashAndTruncate(guid)
			}
		}
	}

	return ""
}

// hashAndTruncate takes a long identifier, hashes it, and returns the first idBytes.
func hashAndTruncate(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	hash := h.Sum(nil)

	if len(hash) < idBytes {
		return ""
	}

	return hex.EncodeToString(hash[:idBytes])
}
